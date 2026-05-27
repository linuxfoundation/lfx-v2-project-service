// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// --- stubs ---

type stubProjectResolver struct {
	sfid string
	err  error
}

func (s *stubProjectResolver) SFIDFromUID(_ context.Context, _ string) (string, error) {
	return s.sfid, s.err
}
func (s *stubProjectResolver) UIDFromSlug(_ context.Context, _ string) (string, error) {
	return "", nil
}

// --- helpers ---

func mustUnmarshal(t *testing.T, payload any) map[string]any {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// --- project handler tests ---

func TestProcessProjectIDMapRequest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		body     string
		resolver *stubProjectResolver
		wantKey  string
		wantVal  string
	}{
		{
			name:     "valid UID returns project_sfid",
			body:     `{"project_uid":"abc-123"}`,
			resolver: &stubProjectResolver{sfid: "a0941000002wBz9AAE"},
			wantKey:  "project_sfid",
			wantVal:  "a0941000002wBz9AAE",
		},
		{
			name:     "missing project_uid returns error",
			body:     `{}`,
			resolver: &stubProjectResolver{},
			wantKey:  "error",
			wantVal:  "project_uid is required",
		},
		{
			name:     "empty project_uid returns error",
			body:     `{"project_uid":""}`,
			resolver: &stubProjectResolver{},
			wantKey:  "error",
			wantVal:  "project_uid is required",
		},
		{
			name:     "resolver error returns not found",
			body:     `{"project_uid":"abc-123"}`,
			resolver: &stubProjectResolver{err: errors.New("not found")},
			wantKey:  "error",
			wantVal:  "project not found",
		},
		{
			name:    "malformed JSON returns invalid request body",
			body:    `not-json`,
			wantKey: "error",
			wantVal: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := tt.resolver
			if resolver == nil {
				resolver = &stubProjectResolver{}
			}
			result := processProjectIDMapRequest(ctx, []byte(tt.body), resolver)
			got := mustUnmarshal(t, result)
			assert.Equal(t, tt.wantVal, got[tt.wantKey], "field %q", tt.wantKey)
		})
	}
}

// --- sfid-to-uuid handler tests ---

func TestProcessSFIDToUUIDRequest(t *testing.T) {
	ctx := context.Background()

	// Use a real 15-char SFID that can be converted
	realSFID := "001B000000IqhSL"
	realUUID, err := sfuuid.ToUUID(realSFID)
	require.NoError(t, err)

	tests := []struct {
		name    string
		body    string
		wantKey string
		wantVal string
	}{
		{
			name:    "valid 15-char SFID returns uuid",
			body:    `{"sfid":"001B000000IqhSL"}`,
			wantKey: "uuid",
			wantVal: realUUID,
		},
		{
			name:    "valid 18-char SFID returns uuid",
			body:    `{"sfid":"001B000000IqhSLIAZ"}`,
			wantKey: "uuid",
			wantVal: realUUID,
		},
		{
			name:    "missing sfid returns error",
			body:    `{}`,
			wantKey: "error",
			wantVal: "sfid is required",
		},
		{
			name:    "empty sfid returns error",
			body:    `{"sfid":""}`,
			wantKey: "error",
			wantVal: "sfid is required",
		},
		{
			name:    "invalid SFID returns error",
			body:    `{"sfid":"invalid!@#$%"}`,
			wantKey: "error",
			wantVal: "invalid sfid",
		},
		{
			name:    "malformed JSON returns invalid request body",
			body:    `not-json`,
			wantKey: "error",
			wantVal: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processSFIDToUUIDRequest(ctx, []byte(tt.body))
			got := mustUnmarshal(t, result)
			assert.Equal(t, tt.wantVal, got[tt.wantKey], "field %q", tt.wantKey)
		})
	}
}

// --- uuid-to-sfid handler tests ---

func TestProcessUUIDToSFIDRequest(t *testing.T) {
	ctx := context.Background()

	// Use a real SFID and convert to UUID for testing
	realSFID := "001B000000IqhSL"
	realUUID, err := sfuuid.ToUUID(realSFID)
	require.NoError(t, err)

	tests := []struct {
		name    string
		body    string
		wantKey string
		wantVal string
	}{
		{
			name:    "valid UUID returns sfid",
			body:    `{"uuid":"` + realUUID + `"}`,
			wantKey: "sfid",
			wantVal: realSFID,
		},
		{
			name:    "missing uuid returns error",
			body:    `{}`,
			wantKey: "error",
			wantVal: "uuid is required",
		},
		{
			name:    "empty uuid returns error",
			body:    `{"uuid":""}`,
			wantKey: "error",
			wantVal: "uuid is required",
		},
		{
			name:    "invalid UUID returns error",
			body:    `{"uuid":"not-a-uuid"}`,
			wantKey: "error",
			wantVal: "invalid uuid",
		},
		{
			name:    "malformed JSON returns invalid request body",
			body:    `not-json`,
			wantKey: "error",
			wantVal: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processUUIDToSFIDRequest(ctx, []byte(tt.body))
			got := mustUnmarshal(t, result)
			assert.Equal(t, tt.wantVal, got[tt.wantKey], "field %q", tt.wantKey)
		})
	}
}
