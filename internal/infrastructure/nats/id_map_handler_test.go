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

type stubB2BOrgResolver struct {
	uid string
	err error
}

func (s *stubB2BOrgResolver) UIDFromSFID(_ context.Context, _ string) (string, error) {
	return s.uid, s.err
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

// --- b2b org handler tests ---

func TestProcessB2BOrgIDMapRequest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		body     string
		resolver *stubB2BOrgResolver
		wantKey  string
		wantVal  string
	}{
		{
			name:     "valid SFID returns b2b_org_uid",
			body:     `{"b2b_org_sfid":"001B000000IqhSLIAZ"}`,
			resolver: &stubB2BOrgResolver{uid: "4c46585f-878c-8019-80e2-5632d301d19b"},
			wantKey:  "b2b_org_uid",
			wantVal:  "4c46585f-878c-8019-80e2-5632d301d19b",
		},
		{
			name:     "missing b2b_org_sfid returns error",
			body:     `{}`,
			resolver: &stubB2BOrgResolver{},
			wantKey:  "error",
			wantVal:  "b2b_org_sfid is required",
		},
		{
			name:     "empty b2b_org_sfid returns error",
			body:     `{"b2b_org_sfid":""}`,
			resolver: &stubB2BOrgResolver{},
			wantKey:  "error",
			wantVal:  "b2b_org_sfid is required",
		},
		{
			name:     "resolver error returns not found",
			body:     `{"b2b_org_sfid":"001B000000IqhSLIAZ"}`,
			resolver: &stubB2BOrgResolver{err: errors.New("not found")},
			wantKey:  "error",
			wantVal:  "b2b org not found",
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
				resolver = &stubB2BOrgResolver{}
			}
			result := processB2BOrgIDMapRequest(ctx, []byte(tt.body), resolver)
			got := mustUnmarshal(t, result)
			assert.Equal(t, tt.wantVal, got[tt.wantKey], "field %q", tt.wantKey)
		})
	}
}
