// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// sfidToUUIDRequest is the JSON request body for the sfid-to-uuid lookup RPC.
type sfidToUUIDRequest struct {
	// SFID is the Salesforce ID to resolve.
	SFID string `json:"sfid"`
}

// sfidToUUIDResponse is the JSON response body for the sfid-to-uuid lookup RPC.
// On success, UUID is populated. On failure, Error is populated.
type sfidToUUIDResponse struct {
	// UUID is the v2 UUID v8 resolved from the SFID.
	UUID string `json:"uuid,omitempty"`
	// Error is a human-readable error message returned when resolution fails.
	Error string `json:"error,omitempty"`
}

// uuidToSFIDRequest is the JSON request body for the uuid-to-sfid lookup RPC.
type uuidToSFIDRequest struct {
	// UUID is the v2 UUID to resolve.
	UUID string `json:"uuid"`
}

// uuidToSFIDResponse is the JSON response body for the uuid-to-sfid lookup RPC.
// On success, SFID is populated. On failure, Error is populated.
type uuidToSFIDResponse struct {
	// SFID is the Salesforce ID resolved from the UUID.
	SFID string `json:"sfid,omitempty"`
	// Error is a human-readable error message returned when resolution fails.
	Error string `json:"error,omitempty"`
}

// processSFIDToUUIDRequest decodes a raw request body and converts the SFID
// to a UUID, returning the response value to be marshalled.
func processSFIDToUUIDRequest(ctx context.Context, data []byte) sfidToUUIDResponse {
	var req sfidToUUIDRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.WarnContext(ctx, "sfid-to-uuid: failed to decode request", "error", err)
		return sfidToUUIDResponse{Error: "invalid request body"}
	}
	if req.SFID == "" {
		return sfidToUUIDResponse{Error: "sfid is required"}
	}
	if !sfuuid.IsSFID(req.SFID) {
		slog.DebugContext(ctx, "sfid-to-uuid: invalid sfid", "sfid", req.SFID)
		return sfidToUUIDResponse{Error: "invalid sfid"}
	}
	u, err := sfuuid.ToUUID(req.SFID)
	if err != nil {
		slog.DebugContext(ctx, "sfid-to-uuid: conversion failed", "sfid", req.SFID, "error", err)
		return sfidToUUIDResponse{Error: "invalid sfid"}
	}
	return sfidToUUIDResponse{UUID: u}
}

// processUUIDToSFIDRequest decodes a raw request body and converts the UUID
// to an SFID, returning the response value to be marshalled.
func processUUIDToSFIDRequest(ctx context.Context, data []byte) uuidToSFIDResponse {
	var req uuidToSFIDRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.WarnContext(ctx, "uuid-to-sfid: failed to decode request", "error", err)
		return uuidToSFIDResponse{Error: "invalid request body"}
	}
	if req.UUID == "" {
		return uuidToSFIDResponse{Error: "uuid is required"}
	}
	// Validate UUID syntax with google/uuid package
	if _, err := uuid.Parse(req.UUID); err != nil {
		slog.DebugContext(ctx, "uuid-to-sfid: invalid uuid", "uuid", req.UUID, "error", err)
		return uuidToSFIDResponse{Error: "invalid uuid"}
	}
	sfid, err := sfuuid.ToSFID(req.UUID)
	if err != nil {
		slog.DebugContext(ctx, "uuid-to-sfid: conversion failed", "uuid", req.UUID, "error", err)
		return uuidToSFIDResponse{Error: "invalid uuid"}
	}
	return uuidToSFIDResponse{SFID: sfid}
}

// HandleSFIDToUUID registers a NATS core request/reply subscription on
// constants.SFIDToUUIDLookupSubject. On each request it converts the Salesforce
// SFID in the JSON body to a v2 UUID v8 and replies with a JSON response.
// Resolution is deterministic (no I/O required).
//
// The returned *nats.Subscription can be used to drain or unsubscribe on
// shutdown. A non-nil error means the subscription could not be established.
func HandleSFIDToUUID(nc *nats.Conn, log *slog.Logger) (*nats.Subscription, error) {
	sub, err := nc.Subscribe(constants.SFIDToUUIDLookupSubject, func(msg *nats.Msg) {
		replyJSON(msg, processSFIDToUUIDRequest(context.Background(), msg.Data))
	})
	if err != nil {
		return nil, err
	}

	log.Info("subscribed to sfid-to-uuid lookup RPC",
		"subject", constants.SFIDToUUIDLookupSubject,
	)

	return sub, nil
}

// HandleUUIDToSFID registers a NATS core request/reply subscription on
// constants.UUIDToSFIDLookupSubject. On each request it converts the v2
// UUID v8 in the JSON body to a Salesforce SFID and replies with a JSON response.
// Resolution is deterministic (no I/O required).
//
// The returned *nats.Subscription can be used to drain or unsubscribe on
// shutdown. A non-nil error means the subscription could not be established.
func HandleUUIDToSFID(nc *nats.Conn, log *slog.Logger) (*nats.Subscription, error) {
	sub, err := nc.Subscribe(constants.UUIDToSFIDLookupSubject, func(msg *nats.Msg) {
		replyJSON(msg, processUUIDToSFIDRequest(context.Background(), msg.Data))
	})
	if err != nil {
		return nil, err
	}

	log.Info("subscribed to uuid-to-sfid lookup RPC",
		"subject", constants.UUIDToSFIDLookupSubject,
	)

	return sub, nil
}
