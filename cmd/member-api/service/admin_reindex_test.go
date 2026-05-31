// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"
	"testing"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Handler tests (AdminReindex endpoint) ──────────────────────────────────

func TestAdminReindex_AcceptsFullReindexAndReturnsRunID(t *testing.T) {
	runner := newTestBackfillRunner(nil)
	svc := newTestSvc(withBackfillRunner(runner))

	result, err := svc.AdminReindex(context.Background(), &membershipservice.AdminReindexPayload{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.RunID, "run_id must be a non-empty UUID")
}

func TestAdminReindex_Validation(t *testing.T) {
	since := "2026-05-01T00:00:00Z"
	offsetSince := "2026-05-01T00:00:00-07:00"
	invalidSince := "not-a-date"
	naiveSince := "2026-05-01 00:00:00"

	tests := []struct {
		name       string
		payload    *membershipservice.AdminReindexPayload
		wantErrMsg string
	}{
		{
			name:       "unknown type rejected",
			payload:    &membershipservice.AdminReindexPayload{Types: []string{"foobar"}},
			wantErrMsg: "unknown type",
		},
		{
			name:       "membership_tier rejected with helpful message",
			payload:    &membershipservice.AdminReindexPayload{Types: []string{"membership_tier"}},
			wantErrMsg: "membership_tier is not currently supported",
		},
		{
			name: "items + types mutually exclusive",
			payload: &membershipservice.AdminReindexPayload{
				Types: []string{"b2b_org"},
				Items: []*membershipservice.AdminReindexItem{{Type: "b2b_org", UID: "00000000-0000-0000-0000-000000000001"}},
			},
			wantErrMsg: "mutually exclusive",
		},
		{
			name: "items + since mutually exclusive",
			payload: &membershipservice.AdminReindexPayload{
				Since: &since,
				Items: []*membershipservice.AdminReindexItem{{Type: "b2b_org", UID: "00000000-0000-0000-0000-000000000001"}},
			},
			wantErrMsg: "mutually exclusive",
		},
		{
			name: "item with invalid UUID rejected",
			payload: &membershipservice.AdminReindexPayload{
				Items: []*membershipservice.AdminReindexItem{{Type: "b2b_org", UID: "not-a-uuid"}},
			},
			wantErrMsg: "invalid UUID",
		},
		{
			name: "item with unknown type rejected",
			payload: &membershipservice.AdminReindexPayload{
				Items: []*membershipservice.AdminReindexItem{{Type: "membership_tier", UID: "00000000-0000-0000-0000-000000000001"}},
			},
			wantErrMsg: "membership_tier is not currently supported in items mode",
		},
		{
			name:       "invalid since rejected",
			payload:    &membershipservice.AdminReindexPayload{Since: &invalidSince},
			wantErrMsg: "RFC 3339",
		},
		{
			name:       "naive since (no zone) rejected",
			payload:    &membershipservice.AdminReindexPayload{Since: &naiveSince},
			wantErrMsg: "RFC 3339",
		},
		{
			name:    "since with non-UTC offset accepted (normalised to UTC)",
			payload: &membershipservice.AdminReindexPayload{Since: &offsetSince},
			// no error
		},
	}

	svc := newTestSvc(withBackfillRunner(newTestBackfillRunner(nil)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AdminReindex(context.Background(), tt.payload)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tt.wantErrMsg),
					"expected error containing %q, got: %v", tt.wantErrMsg, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// newTestBackfillRunner returns a Runner with empty mock iterator (no NATS).
func newTestBackfillRunner(iter usecaseSvc.BackfillIterator) *usecaseSvc.Runner {
	if iter == nil {
		iter = &mock.MockBackfillIterator{}
	}
	return usecaseSvc.NewRunner(iter, mock.NewMockB2BOrgReader(), mock.NewMockProjectMembershipReader(), nil, mock.NewMockB2BOrgSettings(), mock.NewMockMemberPublisher(), nil, "")
}
