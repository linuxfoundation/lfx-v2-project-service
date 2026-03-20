// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── convertTierToResponse ─────────────────────────────────────────────────────

func TestConvertTierToResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		input   *model.MembershipTier
		wantNil bool
	}{
		{
			name:    "nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "full tier maps all fields",
			input: &model.MembershipTier{
				UID:         "tier-uid-1",
				ProjectUID:  "a27394a3-7a6c-4d0f-9e0f-692d8753924f",
				Name:        "Gold Corporate Membership",
				Family:      "Membership",
				ProductType: "Corporate",
				CreatedAt:   now.Add(-48 * time.Hour),
				UpdatedAt:   now,
			},
			wantNil: false,
		},
		{
			name: "tier without optional fields omits them",
			input: &model.MembershipTier{
				UID:       "tier-uid-2",
				ProjectUID: "b1234567-89ab-cdef-0123-456789abcdef",
				Name:      "Silver Membership",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTierToResponse(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			require.NotNil(t, result.UID)
			assert.Equal(t, tt.input.UID, *result.UID)

			require.NotNil(t, result.ProjectUID)
			assert.Equal(t, tt.input.ProjectUID, *result.ProjectUID)

			require.NotNil(t, result.Name)
			assert.Equal(t, tt.input.Name, *result.Name)

			if tt.input.Family != "" {
				require.NotNil(t, result.Family)
				assert.Equal(t, tt.input.Family, *result.Family)
			} else {
				assert.Nil(t, result.Family)
			}

			if tt.input.ProductType != "" {
				require.NotNil(t, result.ProductType)
				assert.Equal(t, tt.input.ProductType, *result.ProductType)
			} else {
				assert.Nil(t, result.ProductType)
			}

			if !tt.input.CreatedAt.IsZero() {
				require.NotNil(t, result.CreatedAt)
				assert.NotEmpty(t, *result.CreatedAt)
			}
			if !tt.input.UpdatedAt.IsZero() {
				require.NotNil(t, result.UpdatedAt)
				assert.NotEmpty(t, *result.UpdatedAt)
			}
		})
	}
}

// ── convertProjectMembershipToResponse ───────────────────────────────────────

func TestConvertProjectMembershipToResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		input   *model.ProjectMembership
		wantNil bool
	}{
		{
			name:    "nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "full membership maps all fields",
			input: &model.ProjectMembership{
				UID:              "membership-uid-1",
				TierUID:          "tier-uid-1",
				ProjectUID:       "a27394a3-7a6c-4d0f-9e0f-692d8753924f",
				Status:           "Active",
				Year:             "2025",
				Tier:             "Gold",
				MembershipType:   "Corporate",
				AutoRenew:        true,
				RenewalType:      "Annual",
				Price:            50000.00,
				AnnualFullPrice:  50000.00,
				PaymentFrequency: "Annual",
				PaymentTerms:     "Net 30",
				AgreementDate:    "2025-01-15T00:00:00Z",
				PurchaseDate:     "2025-01-20T00:00:00Z",
				StartDate:        "2025-02-01T00:00:00Z",
				EndDate:          "2025-12-31T23:59:59Z",
				CompanyName:      "Example Corp",
				CompanyLogoURL:   "https://example.com/logo.png",
				CompanyDomain:    "https://example.com",
				TierName:         "Gold Corporate Membership",
				TierFamily:       "Membership",
				TierProductType:  "Corporate",
				CreatedAt:        now.Add(-24 * time.Hour),
				UpdatedAt:        now,
			},
			wantNil: false,
		},
		{
			name: "membership with only required fields",
			input: &model.ProjectMembership{
				UID:            "membership-uid-2",
				TierUID:        "tier-uid-2",
				ProjectUID:     "b1234567-89ab-cdef-0123-456789abcdef",
				Status:         "Expired",
				MembershipType: "Corporate",
				AutoRenew:      false,
				CompanyName:    "Minimal Corp",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProjectMembershipToResponse(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)

			require.NotNil(t, result.UID)
			assert.Equal(t, tt.input.UID, *result.UID)

			require.NotNil(t, result.TierUID)
			assert.Equal(t, tt.input.TierUID, *result.TierUID)

			require.NotNil(t, result.ProjectUID)
			assert.Equal(t, tt.input.ProjectUID, *result.ProjectUID)

			require.NotNil(t, result.Status)
			assert.Equal(t, tt.input.Status, *result.Status)

			require.NotNil(t, result.MembershipType)
			assert.Equal(t, tt.input.MembershipType, *result.MembershipType)

			require.NotNil(t, result.AutoRenew)
			assert.Equal(t, tt.input.AutoRenew, *result.AutoRenew)

			require.NotNil(t, result.CompanyName)
			assert.Equal(t, tt.input.CompanyName, *result.CompanyName)

			if tt.input.Year != "" {
				require.NotNil(t, result.Year)
				assert.Equal(t, tt.input.Year, *result.Year)
			} else {
				assert.Nil(t, result.Year)
			}

			if tt.input.Tier != "" {
				require.NotNil(t, result.Tier)
				assert.Equal(t, tt.input.Tier, *result.Tier)
			} else {
				assert.Nil(t, result.Tier)
			}

			if tt.input.CompanyLogoURL != "" {
				require.NotNil(t, result.CompanyLogoURL)
				assert.Equal(t, tt.input.CompanyLogoURL, *result.CompanyLogoURL)
			} else {
				assert.Nil(t, result.CompanyLogoURL)
			}

			if tt.input.CompanyDomain != "" {
				require.NotNil(t, result.CompanyDomain)
				assert.Equal(t, tt.input.CompanyDomain, *result.CompanyDomain)
			} else {
				assert.Nil(t, result.CompanyDomain)
			}

			if tt.input.TierName != "" {
				require.NotNil(t, result.TierName)
				assert.Equal(t, tt.input.TierName, *result.TierName)
			} else {
				assert.Nil(t, result.TierName)
			}

			if tt.input.TierFamily != "" {
				require.NotNil(t, result.TierFamily)
				assert.Equal(t, tt.input.TierFamily, *result.TierFamily)
			} else {
				assert.Nil(t, result.TierFamily)
			}

			if tt.input.TierProductType != "" {
				require.NotNil(t, result.TierProductType)
				assert.Equal(t, tt.input.TierProductType, *result.TierProductType)
			} else {
				assert.Nil(t, result.TierProductType)
			}
		})
	}
}

// ── convertProjectKeyContactToResponse ───────────────────────────────────────

func TestConvertProjectKeyContactToResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		input   *model.ProjectKeyContact
		wantNil bool
	}{
		{
			name:    "nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "full key contact maps all fields",
			input: &model.ProjectKeyContact{
				UID:            "kc-uid-1",
				MembershipUID:  "membership-uid-1",
				TierUID:        "tier-uid-1",
				ProjectUID:     "a27394a3-7a6c-4d0f-9e0f-692d8753924f",
				Role:           "Voting Representative",
				Status:         "Active",
				BoardMember:    false,
				PrimaryContact: true,
				FirstName:      "John",
				LastName:       "Doe",
				Title:          "CTO",
				Email:          "john.doe@example.com",
				CompanyName:    "Example Corp",
				CompanyLogoURL: "https://example.com/logo.png",
				CompanyDomain:  "https://example.com",
				CreatedAt:      now.Add(-24 * time.Hour),
				UpdatedAt:      now,
			},
			wantNil: false,
		},
		{
			name: "key contact without optional fields",
			input: &model.ProjectKeyContact{
				UID:           "kc-uid-2",
				MembershipUID: "membership-uid-2",
				TierUID:       "tier-uid-2",
				ProjectUID:    "b1234567-89ab-cdef-0123-456789abcdef",
				Role:          "Billing Contact",
				Status:        "Active",
				FirstName:     "Jane",
				LastName:      "Smith",
				CompanyName:   "Minimal Corp",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProjectKeyContactToResponse(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)

			require.NotNil(t, result.UID)
			assert.Equal(t, tt.input.UID, *result.UID)

			require.NotNil(t, result.MembershipUID)
			assert.Equal(t, tt.input.MembershipUID, *result.MembershipUID)

			require.NotNil(t, result.TierUID)
			assert.Equal(t, tt.input.TierUID, *result.TierUID)

			require.NotNil(t, result.ProjectUID)
			assert.Equal(t, tt.input.ProjectUID, *result.ProjectUID)

			require.NotNil(t, result.Role)
			assert.Equal(t, tt.input.Role, *result.Role)

			require.NotNil(t, result.Status)
			assert.Equal(t, tt.input.Status, *result.Status)

			require.NotNil(t, result.BoardMember)
			assert.Equal(t, tt.input.BoardMember, *result.BoardMember)

			require.NotNil(t, result.PrimaryContact)
			assert.Equal(t, tt.input.PrimaryContact, *result.PrimaryContact)

			require.NotNil(t, result.FirstName)
			assert.Equal(t, tt.input.FirstName, *result.FirstName)

			require.NotNil(t, result.LastName)
			assert.Equal(t, tt.input.LastName, *result.LastName)

			require.NotNil(t, result.CompanyName)
			assert.Equal(t, tt.input.CompanyName, *result.CompanyName)

			if tt.input.Title != "" {
				require.NotNil(t, result.Title)
				assert.Equal(t, tt.input.Title, *result.Title)
			} else {
				assert.Nil(t, result.Title)
			}

			if tt.input.Email != "" {
				require.NotNil(t, result.Email)
				assert.Equal(t, tt.input.Email, *result.Email)
			} else {
				assert.Nil(t, result.Email)
			}

			if tt.input.CompanyLogoURL != "" {
				require.NotNil(t, result.CompanyLogoURL)
				assert.Equal(t, tt.input.CompanyLogoURL, *result.CompanyLogoURL)
			} else {
				assert.Nil(t, result.CompanyLogoURL)
			}

			if tt.input.CompanyDomain != "" {
				require.NotNil(t, result.CompanyDomain)
				assert.Equal(t, tt.input.CompanyDomain, *result.CompanyDomain)
			} else {
				assert.Nil(t, result.CompanyDomain)
			}

			if !tt.input.CreatedAt.IsZero() {
				require.NotNil(t, result.CreatedAt)
				assert.NotEmpty(t, *result.CreatedAt)
			}
			if !tt.input.UpdatedAt.IsZero() {
				require.NotNil(t, result.UpdatedAt)
				assert.NotEmpty(t, *result.UpdatedAt)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// mustStrPtr panics if s is empty. Used in table-driven tests where an empty
// value would indicate a bug in the test data itself.
func mustStrPtr(t *testing.T, s string) *string {
	t.Helper()
	require.NotEmpty(t, s, "mustStrPtr: value must not be empty")
	return &s
}
