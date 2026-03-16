// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package nats provides NATS JetStream KV-backed implementations of the domain
// storage ports.
package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

// projectIDMapRequest is the JSON request body for the project-id-map lookup RPC.
type projectIDMapRequest struct {
	// ProjectUID is the v2 project UID to resolve.
	ProjectUID string `json:"project_uid"`
}

// projectIDMapResponse is the JSON response body for the project-id-map lookup
// RPC. On success, ProjectSFID is populated. On failure, Error is populated.
type projectIDMapResponse struct {
	// ProjectSFID is the Salesforce Project__c.Id resolved from the UID.
	ProjectSFID string `json:"project_sfid,omitempty"`
	// Error is a human-readable error message returned when resolution fails.
	Error string `json:"error,omitempty"`
}

// SubscribeProjectIDMap registers a NATS core request/reply subscription on
// constants.ProjectIDMapLookupSubject. On each request it resolves the v2
// project UID in the JSON body to a Salesforce Project__c.Id using the supplied
// resolver and replies with a JSON response. The subscription is synchronous per
// message (no queue group); callers that want load-balanced processing should
// pass a queue group via NATS options instead.
//
// The returned *nats.Subscription can be used to drain or unsubscribe on
// shutdown. A non-nil error means the subscription could not be established.
func SubscribeProjectIDMap(conn *nats.Conn, resolver port.ProjectResolver) (*nats.Subscription, error) {
	sub, err := conn.Subscribe(constants.ProjectIDMapLookupSubject, func(msg *nats.Msg) {
		ctx := context.Background()

		var req projectIDMapRequest
		if decodeErr := json.Unmarshal(msg.Data, &req); decodeErr != nil {
			slog.WarnContext(ctx, "project-id-map: failed to decode request",
				"error", decodeErr,
			)
			replyError(msg, "invalid request body")
			return
		}

		if req.ProjectUID == "" {
			replyError(msg, "project_uid is required")
			return
		}

		sfid, resolveErr := resolver.SFIDFromUID(ctx, req.ProjectUID)
		if resolveErr != nil {
			slog.DebugContext(ctx, "project-id-map: resolution failed",
				"project_uid", req.ProjectUID,
				"error", resolveErr,
			)
			replyError(msg, "project not found")
			return
		}

		replyJSON(msg, projectIDMapResponse{ProjectSFID: sfid})
	})
	if err != nil {
		return nil, err
	}

	slog.Info("subscribed to project-id-map lookup RPC",
		"subject", constants.ProjectIDMapLookupSubject,
	)

	return sub, nil
}

// replyError sends a JSON error response to the NATS reply subject.
func replyError(msg *nats.Msg, errMsg string) {
	replyJSON(msg, projectIDMapResponse{Error: errMsg})
}

// replyJSON marshals resp and publishes it to msg.Reply. Marshalling errors are
// logged and a plain-text fallback is sent instead so the caller never hangs
// waiting for a reply that never arrives.
func replyJSON(msg *nats.Msg, resp projectIDMapResponse) {
	if msg.Reply == "" {
		// Fire-and-forget message; nothing to reply to.
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("project-id-map: failed to marshal response", "error", err)
		_ = msg.Respond([]byte(`{"error":"internal serialisation error"}`))
		return
	}

	if err := msg.Respond(data); err != nil {
		slog.Error("project-id-map: failed to send reply", "error", err)
	}
}
