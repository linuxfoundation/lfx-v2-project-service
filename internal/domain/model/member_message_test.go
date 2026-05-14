// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"context"
	"encoding/json"
	"testing"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authCtx returns a context with both auth and principal values set.
func authCtx() context.Context {
	ctx := context.WithValue(context.Background(), constants.AuthorizationContextID, "Bearer test-token")
	return context.WithValue(ctx, constants.PrincipalContextID, "test-principal")
}

// TestMemberIndexerMessage_Build_Created verifies the full wire format for a
// created action: headers injected from context, data decoded to map[string]any.
func TestMemberIndexerMessage_Build_Created(t *testing.T) {
	org := &B2BOrg{
		UID:  "b2b-org-uid-001",
		SFID: "001000000000001AAA",
		Name: "Linux Foundation",
	}

	msg := &MemberIndexerMessage{Action: indexerConstants.ActionCreated}
	built, err := msg.Build(authCtx(), org)

	require.NoError(t, err)
	require.NotNil(t, built)

	// Headers must carry auth and on-behalf-of from context.
	assert.Equal(t, "Bearer test-token", built.Headers[constants.AuthorizationHeader])
	assert.Equal(t, "test-principal", built.Headers[constants.XOnBehalfOfHeader])

	// Data must be a map (not a struct) — required by the indexer wire format.
	dataMap, ok := built.Data.(map[string]any)
	require.True(t, ok, "Data must be map[string]any for created action, got %T", built.Data)
	assert.Equal(t, "b2b-org-uid-001", dataMap["uid"])
	assert.Equal(t, "Linux Foundation", dataMap["name"])
}

// TestMemberIndexerMessage_Build_Deleted verifies that deleted actions carry
// the UID string directly as Data, not a decoded map.
func TestMemberIndexerMessage_Build_Deleted(t *testing.T) {
	msg := &MemberIndexerMessage{Action: indexerConstants.ActionDeleted}
	built, err := msg.Build(authCtx(), "b2b-org-uid-001")

	require.NoError(t, err)
	assert.Equal(t, "b2b-org-uid-001", built.Data,
		"deleted action must carry UID string as Data")
}

// TestMemberIndexerMessage_Build_AuthFallback verifies that when no auth token
// is in context the service-account bearer is used as fallback.
func TestMemberIndexerMessage_Build_AuthFallback(t *testing.T) {
	msg := &MemberIndexerMessage{Action: indexerConstants.ActionUpdated}
	built, err := msg.Build(context.Background(), &B2BOrg{UID: "uid"})

	require.NoError(t, err)
	assert.Equal(t, constants.ServiceAccountBearer, built.Headers[constants.AuthorizationHeader],
		"fallback to service-account bearer when context has no auth token")
	assert.Empty(t, built.Headers[constants.XOnBehalfOfHeader],
		"x-on-behalf-of must be absent when context has no principal")
}

// TestMemberIndexerMessage_Build_JSONRoundTrip verifies that the full
// MemberIndexerMessage serialises to JSON without any fields being dropped.
// This locks down the wire format for lfx-v2-indexer-service consumers.
func TestMemberIndexerMessage_Build_JSONRoundTrip(t *testing.T) {
	org := &B2BOrg{
		UID:  "b2b-org-uid-001",
		Name: "Linux Foundation",
	}
	msg := &MemberIndexerMessage{
		Action: indexerConstants.ActionCreated,
		Tags:   org.Tags(),
	}
	built, err := msg.Build(authCtx(), org)
	require.NoError(t, err)

	data, err := json.Marshal(built)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "created", decoded["action"])
	assert.NotNil(t, decoded["headers"])
	assert.NotNil(t, decoded["data"])
	assert.NotNil(t, decoded["tags"])
	// indexing_config is optional and not set here — must be omitted from JSON.
	assert.Nil(t, decoded["indexing_config"],
		"indexing_config must be omitted when nil (json:omitempty)")
}
