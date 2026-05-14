// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/go-viper/mapstructure/v2"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
)

// MemberIndexerMessage is the NATS message envelope for member-service indexing
// and FGA-sync events. It matches the wire format expected by lfx-v2-indexer-service.
type MemberIndexerMessage struct {
	Action         indexerConstants.MessageAction `json:"action"`
	Headers        map[string]string              `json:"headers"`
	Data           any                            `json:"data"`
	Tags           []string                       `json:"tags,omitempty"`
	IndexingConfig *indexerTypes.IndexingConfig   `json:"indexing_config,omitempty"`
}

// Build populates the message with authorization headers from ctx and converts
// input into the payload format expected by the indexer service.
//
// For created/updated actions, input is JSON-marshalled then mapstructure-decoded
// into map[string]any so the indexer receives a JSON object rather than a quoted
// blob. For deleted actions, input is used as-is (typically the UID string).
//
// The authorization header falls back to constants.ServiceAccountBearer when the
// context carries no user token — this matches the meeting-service convention.
func (m *MemberIndexerMessage) Build(ctx context.Context, input any) (*MemberIndexerMessage, error) {
	headers := make(map[string]string)

	if auth, ok := ctx.Value(constants.AuthorizationContextID).(string); ok && auth != "" {
		headers[constants.AuthorizationHeader] = auth
	} else {
		headers[constants.AuthorizationHeader] = constants.ServiceAccountBearer
	}

	if principal, ok := ctx.Value(constants.PrincipalContextID).(string); ok && principal != "" {
		headers[constants.XOnBehalfOfHeader] = principal
	}

	m.Headers = headers

	switch m.Action {
	case indexerConstants.ActionCreated, indexerConstants.ActionUpdated:
		data, err := json.Marshal(input)
		if err != nil {
			slog.ErrorContext(ctx, "member indexer message: marshal failed", "error", err)
			return nil, fmt.Errorf("member indexer message: marshal: %w", err)
		}
		var jsonData any
		if err := json.Unmarshal(data, &jsonData); err != nil {
			slog.ErrorContext(ctx, "member indexer message: unmarshal failed", "error", err)
			return nil, fmt.Errorf("member indexer message: unmarshal: %w", err)
		}
		cfg := mapstructure.DecoderConfig{TagName: "json", Result: &m.Data}
		dec, err := mapstructure.NewDecoder(&cfg)
		if err != nil {
			return nil, fmt.Errorf("member indexer message: decoder: %w", err)
		}
		if err := dec.Decode(jsonData); err != nil {
			slog.ErrorContext(ctx, "member indexer message: decode failed", "error", err)
			return nil, fmt.Errorf("member indexer message: decode: %w", err)
		}
	case indexerConstants.ActionDeleted:
		// Deleted messages carry only the UID string as data.
		m.Data = input
	}

	return m, nil
}
