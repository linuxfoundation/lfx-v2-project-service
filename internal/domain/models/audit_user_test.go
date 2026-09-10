// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneUserInfo(t *testing.T) {
	expires := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   *UserInfo
	}{
		{
			name: "nil input returns nil",
			in:   nil,
		},
		{
			name: "user without invite",
			in:   &UserInfo{Username: "alice", Name: "Alice Example", Email: "alice@example.com", Avatar: "https://example.com/alice.png"},
		},
		{
			name: "user with invite",
			in: &UserInfo{
				Username: "bob",
				Email:    "bob@example.com",
				Invite:   &InviteInfo{UID: "invite-001", Email: "bob@example.com", ExpiresAt: &expires},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CloneUserInfo(tt.in)

			if tt.in == nil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			// Value equality.
			assert.Equal(t, *tt.in, *got)
			// Must be a different pointer.
			assert.NotSame(t, tt.in, got)

			if tt.in.Invite != nil {
				require.NotNil(t, got.Invite)
				// Invite must be a deep copy — mutating the clone must not affect the original.
				assert.NotSame(t, tt.in.Invite, got.Invite)
				got.Invite.UID = "mutated"
				assert.Equal(t, "invite-001", tt.in.Invite.UID,
					"mutating clone's Invite must not affect original")
			} else {
				assert.Nil(t, got.Invite)
			}
		})
	}
}

func TestNormalizeLegacyAuditUsers(t *testing.T) {
	alice := &UserInfo{Username: "alice"}
	bob := &UserInfo{Username: "bob"}

	tests := []struct {
		name                     string
		createdBy                *UserInfo
		updatedBy                *UserInfo
		legacyCreatedByUsername  string
		legacyUploadedByUsername string
		wantCreatedByUsername    string // empty means expect nil
		wantUpdatedByUsername    string // empty means expect nil
	}{
		{
			name:                  "both nil, no legacy fields — all remain nil",
			wantCreatedByUsername: "",
			wantUpdatedByUsername: "",
		},
		{
			name:                    "both nil, legacyCreatedByUsername set",
			legacyCreatedByUsername: "legacy-creator",
			wantCreatedByUsername:   "legacy-creator",
			wantUpdatedByUsername:   "legacy-creator", // updatedBy defaults to clone of createdBy
		},
		{
			name:                     "both nil, only legacyUploadedByUsername set",
			legacyUploadedByUsername: "uploader",
			wantCreatedByUsername:    "uploader",
			wantUpdatedByUsername:    "uploader",
		},
		{
			name:                     "both nil, both legacy fields set — legacyCreatedByUsername wins",
			legacyCreatedByUsername:  "creator",
			legacyUploadedByUsername: "uploader",
			wantCreatedByUsername:    "creator",
			wantUpdatedByUsername:    "creator",
		},
		{
			name:                    "createdBy already set — not overwritten by legacy field",
			createdBy:               alice,
			legacyCreatedByUsername: "should-be-ignored",
			wantCreatedByUsername:   "alice",
			wantUpdatedByUsername:   "alice", // updatedBy nil → set to clone of createdBy
		},
		{
			name:                    "both already set — neither overwritten",
			createdBy:               alice,
			updatedBy:               bob,
			legacyCreatedByUsername: "should-be-ignored",
			wantCreatedByUsername:   "alice",
			wantUpdatedByUsername:   "bob",
		},
		{
			name:                  "createdBy set, updatedBy already set — updatedBy kept as-is",
			createdBy:             alice,
			updatedBy:             bob,
			wantCreatedByUsername: "alice",
			wantUpdatedByUsername: "bob",
		},
		{
			name:                    "whitespace-only legacy username treated as empty",
			legacyCreatedByUsername: "   ",
			wantCreatedByUsername:   "",
			wantUpdatedByUsername:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCreated, gotUpdated := NormalizeLegacyAuditUsers(
				tt.createdBy, tt.updatedBy,
				tt.legacyCreatedByUsername, tt.legacyUploadedByUsername,
			)

			if tt.wantCreatedByUsername == "" {
				assert.Nil(t, gotCreated)
			} else {
				require.NotNil(t, gotCreated)
				assert.Equal(t, tt.wantCreatedByUsername, gotCreated.Username)
			}

			if tt.wantUpdatedByUsername == "" {
				assert.Nil(t, gotUpdated)
			} else {
				require.NotNil(t, gotUpdated)
				assert.Equal(t, tt.wantUpdatedByUsername, gotUpdated.Username)
			}
		})
	}
}

func TestAuditCreatorUsername(t *testing.T) {
	tests := []struct {
		name      string
		createdBy *UserInfo
		want      string
	}{
		{
			name:      "nil returns empty string",
			createdBy: nil,
			want:      "",
		},
		{
			name:      "empty username returns empty string",
			createdBy: &UserInfo{Username: ""},
			want:      "",
		},
		{
			name:      "whitespace-only username returns empty string",
			createdBy: &UserInfo{Username: "   "},
			want:      "",
		},
		{
			name:      "valid username returned as-is",
			createdBy: &UserInfo{Username: "alice"},
			want:      "alice",
		},
		{
			name:      "username with surrounding whitespace is trimmed",
			createdBy: &UserInfo{Username: "  bob  "},
			want:      "bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AuditCreatorUsername(tt.createdBy))
		})
	}
}

func TestAuditUserNeedsMigration(t *testing.T) {
	tests := []struct {
		name string
		u    *UserInfo
		want bool
	}{
		{
			name: "nil — no migration needed",
			u:    nil,
			want: false,
		},
		{
			name: "empty username — skip (cannot look up without LFID)",
			u:    &UserInfo{},
			want: false,
		},
		{
			name: "whitespace-only username — treated as empty, skip",
			u:    &UserInfo{Username: "   "},
			want: false,
		},
		{
			name: "username set, Name already populated — no migration needed",
			u:    &UserInfo{Username: "alice", Name: "Alice Example"},
			want: false,
		},
		{
			name: "username set, Name missing — migration needed",
			u:    &UserInfo{Username: "bob"},
			want: true,
		},
		{
			name: "username set, Name is whitespace only — migration needed",
			u:    &UserInfo{Username: "carol", Name: "   "},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AuditUserNeedsMigration(tt.u))
		})
	}
}
