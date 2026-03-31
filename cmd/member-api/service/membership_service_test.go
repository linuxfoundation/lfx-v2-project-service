// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"testing"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() membershipservice.Service {
	mockRepo := mock.NewMockMembershipRepository()
	orchestrator := usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(mockRepo),
	)
	jwtAuth, _ := auth.NewJWTAuth(auth.JWTAuthConfig{
		MockLocalPrincipal: "test-user",
	})
	return NewMembershipService(orchestrator, mockRepo, jwtAuth, nil)
}

// ── Tiers ─────────────────────────────────────────────────────────────────────

func TestListProjectTiers(t *testing.T) {
	tests := []struct {
		name      string
		payload   *membershipservice.ListProjectTiersPayload
		wantErr   bool
		wantCount int
	}{
		{
			name: "list tiers for project with sample data",
			payload: &membershipservice.ListProjectTiersPayload{
				ProjectUID: strPtr("project-uid-1"),
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "list tiers for project with no data",
			payload: &membershipservice.ListProjectTiersPayload{
				ProjectUID: strPtr("nonexistent-project"),
			},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.ListProjectTiers(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Len(t, res.Tiers, tt.wantCount)
		})
	}
}

func TestGetProjectTier(t *testing.T) {
	tests := []struct {
		name    string
		payload *membershipservice.GetProjectTierPayload
		wantErr bool
	}{
		{
			name: "get existing tier",
			payload: &membershipservice.GetProjectTierPayload{
				ProjectUID: strPtr("project-uid-1"),
				TierUID:    strPtr("tier-1"),
			},
			wantErr: false,
		},
		{
			name: "get nonexistent tier",
			payload: &membershipservice.GetProjectTierPayload{
				ProjectUID: strPtr("project-uid-1"),
				TierUID:    strPtr("nonexistent-tier"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.GetProjectTier(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			require.NotNil(t, res.Tier)
			assert.Equal(t, "tier-1", *res.Tier.UID)
		})
	}
}

// ── Memberships ───────────────────────────────────────────────────────────────

func TestListProjectMemberships(t *testing.T) {
	tests := []struct {
		name      string
		payload   *membershipservice.ListProjectMembershipsPayload
		wantErr   bool
		wantCount int
		wantTotal int
	}{
		{
			name: "list memberships for project with sample data",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships for project with no data",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("nonexistent-project"),
				PageSize:   25,
				Sort:       "newest",
			},
			wantErr:   false,
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name: "list memberships with status filter matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				Filter:     strPtr("status=Active"),
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships with status filter non-matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				Filter:     strPtr("status=Expired"),
			},
			wantErr:   false,
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name: "list memberships with tier filter matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				Filter:     strPtr("tier=Gold"),
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships with project_slug filter matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				Filter:     strPtr("project_slug=linux-foundation"),
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships with project_slug filter non-matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				Filter:     strPtr("project_slug=non-existent"),
			},
			wantErr:   false,
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name: "list memberships with search_name matching company name",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				SearchName: strPtr("Example"),
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships with search_name non-matching",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				SearchName: strPtr("NonExistent"),
			},
			wantErr:   false,
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name: "list memberships sort by name",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "name",
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships sort by last_modified",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "last_modified",
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name: "list memberships with page token (continuation)",
			payload: &membershipservice.ListProjectMembershipsPayload{
				ProjectUID: strPtr("project-uid-1"),
				PageSize:   25,
				Sort:       "newest",
				PageToken:  strPtr(""),
			},
			wantErr:   false,
			wantCount: 1,
			wantTotal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.ListProjectMemberships(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Len(t, res.Memberships, tt.wantCount)
			if tt.wantTotal > 0 {
				require.NotNil(t, res.Metadata.TotalSize, "expected non-nil TotalSize")
				assert.Equal(t, tt.wantTotal, *res.Metadata.TotalSize)
			}
		})
	}
}

func TestGetProjectMembership(t *testing.T) {
	tests := []struct {
		name    string
		payload *membershipservice.GetProjectMembershipPayload
		wantErr bool
	}{
		{
			name: "get existing membership for project",
			payload: &membershipservice.GetProjectMembershipPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("membership-1"),
			},
			wantErr: false,
		},
		{
			name: "get nonexistent membership",
			payload: &membershipservice.GetProjectMembershipPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("nonexistent"),
			},
			wantErr: true,
		},
		{
			name: "get membership belonging to different project",
			payload: &membershipservice.GetProjectMembershipPayload{
				ProjectUID:    strPtr("wrong-project"),
				MembershipUID: strPtr("membership-1"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.GetProjectMembership(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			require.NotNil(t, res.Membership)
			assert.Equal(t, "membership-1", *res.Membership.UID)
		})
	}
}

// ── Key contacts ──────────────────────────────────────────────────────────────

func TestListMembershipKeyContacts(t *testing.T) {
	tests := []struct {
		name      string
		payload   *membershipservice.ListMembershipKeyContactsPayload
		wantErr   bool
		wantCount int
	}{
		{
			name: "list contacts for existing membership",
			payload: &membershipservice.ListMembershipKeyContactsPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("membership-1"),
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "list contacts for membership with no contacts",
			payload: &membershipservice.ListMembershipKeyContactsPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("nonexistent-membership"),
			},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.ListMembershipKeyContacts(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Len(t, res.Contacts, tt.wantCount)
		})
	}
}

func TestGetMembershipKeyContact(t *testing.T) {
	tests := []struct {
		name    string
		payload *membershipservice.GetMembershipKeyContactPayload
		wantErr bool
	}{
		{
			name: "get existing key contact",
			payload: &membershipservice.GetMembershipKeyContactPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("membership-1"),
				ContactUID:    strPtr("contact-role-1"),
			},
			wantErr: false,
		},
		{
			name: "get nonexistent key contact",
			payload: &membershipservice.GetMembershipKeyContactPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("membership-1"),
				ContactUID:    strPtr("nonexistent-contact"),
			},
			wantErr: true,
		},
		{
			name: "get key contact belonging to different membership",
			payload: &membershipservice.GetMembershipKeyContactPayload{
				ProjectUID:    strPtr("project-uid-1"),
				MembershipUID: strPtr("wrong-membership"),
				ContactUID:    strPtr("contact-role-1"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			ctx := context.Background()

			res, err := svc.GetMembershipKeyContact(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			require.NotNil(t, res.Contact)
			assert.Equal(t, "contact-role-1", *res.Contact.UID)
		})
	}
}

// ── Health probes ─────────────────────────────────────────────────────────────

func TestReadyz(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	res, err := svc.Readyz(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("OK\n"), res)
}

func TestLivez(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	res, err := svc.Livez(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte("OK\n"), res)
}

func TestDebugVars(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	res, err := svc.DebugVars(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, res)

	// Output must be valid JSON.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(res, &parsed), "DebugVars output must be valid JSON")

	// The default expvar registry always includes cmdline and memstats.
	assert.Contains(t, parsed, "cmdline")
	assert.Contains(t, parsed, "memstats")
}

// ── Utility ───────────────────────────────────────────────────────────────────

func TestParseFilters(t *testing.T) {
	tests := []struct {
		name   string
		filter *string
		want   map[string]string
	}{
		{
			name:   "nil filter returns empty map",
			filter: nil,
			want:   map[string]string{},
		},
		{
			name:   "empty filter returns empty map",
			filter: strPtr(""),
			want:   map[string]string{},
		},
		{
			name:   "single key=value pair",
			filter: strPtr("status=Active"),
			want:   map[string]string{"status": "Active"},
		},
		{
			name:   "multiple key=value pairs separated by semicolons",
			filter: strPtr("status=Active;tier=Gold"),
			want:   map[string]string{"status": "Active", "tier": "Gold"},
		},
		{
			name:   "value with spaces trimmed",
			filter: strPtr("status = Active ; tier = Gold"),
			want:   map[string]string{"status": "Active", "tier": "Gold"},
		},
		{
			name:   "pair without equals sign is ignored",
			filter: strPtr("status=Active;badpair"),
			want:   map[string]string{"status": "Active"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFilters(tt.filter)
			assert.Equal(t, tt.want, result)
		})
	}
}
