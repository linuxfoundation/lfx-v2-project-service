// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

// b2bOrgIDMapRequest is the JSON request body for the b2b-org-id-map lookup RPC.
type b2bOrgIDMapRequest struct {
	// B2BOrgSFID is the Salesforce Account.Id to resolve.
	B2BOrgSFID string `json:"b2b_org_sfid"`
}

// b2bOrgIDMapResponse is the JSON response body for the b2b-org-id-map lookup
// RPC. On success, B2BOrgUID is populated. On failure, Error is populated.
type b2bOrgIDMapResponse struct {
	// B2BOrgUID is the v2 b2b_org UUID resolved from the SFID.
	B2BOrgUID string `json:"b2b_org_uid,omitempty"`
	// Error is a human-readable error message returned when resolution fails.
	Error string `json:"error,omitempty"`
}

// SubscribeB2BOrgIDMap registers a NATS core request/reply subscription on
// constants.B2BOrgIDMapLookupSubject. On each request it resolves the Salesforce
// Account SFID in the JSON body to a v2 b2b_org UUID using the supplied resolver
// and replies with a JSON response.
//
// The returned *nats.Subscription can be used to drain or unsubscribe on
// shutdown. A non-nil error means the subscription could not be established.
// processB2BOrgIDMapRequest decodes a raw request body and calls the resolver,
// returning the response value to be marshalled. Extracted for testability.
func processB2BOrgIDMapRequest(ctx context.Context, data []byte, resolver port.B2BOrgResolver) any {
	var req b2bOrgIDMapRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.WarnContext(ctx, "b2b-org-id-map: failed to decode request", "error", err)
		return map[string]string{"error": "invalid request body"}
	}
	if req.B2BOrgSFID == "" {
		return map[string]string{"error": "b2b_org_sfid is required"}
	}
	uid, err := resolver.UIDFromSFID(ctx, req.B2BOrgSFID)
	if err != nil {
		slog.DebugContext(ctx, "b2b-org-id-map: resolution failed",
			"b2b_org_sfid", req.B2BOrgSFID, "error", err)
		return map[string]string{"error": "b2b org not found"}
	}
	return b2bOrgIDMapResponse{B2BOrgUID: uid}
}

func SubscribeB2BOrgIDMap(conn *nats.Conn, resolver port.B2BOrgResolver) (*nats.Subscription, error) {
	sub, err := conn.Subscribe(constants.B2BOrgIDMapLookupSubject, func(msg *nats.Msg) {
		replyJSON(msg, processB2BOrgIDMapRequest(context.Background(), msg.Data, resolver))
	})
	if err != nil {
		return nil, err
	}

	slog.Info("subscribed to b2b-org-id-map lookup RPC",
		"subject", constants.B2BOrgIDMapLookupSubject,
	)

	return sub, nil
}
