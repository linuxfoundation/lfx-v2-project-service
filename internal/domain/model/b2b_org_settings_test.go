// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestB2BOrgUser_EffectiveStatus(t *testing.T) {
	tests := []struct {
		name string
		user B2BOrgUser
		want InviteStatus
	}{
		{
			name: "explicit accepted returned as-is",
			user: B2BOrgUser{Username: "alice", InviteStatus: InviteStatusAccepted},
			want: InviteStatusAccepted,
		},
		{
			name: "explicit pending returned as-is",
			user: B2BOrgUser{Email: "bob@example.com", InviteStatus: InviteStatusPending},
			want: InviteStatusPending,
		},
		{
			name: "explicit revoked returned as-is",
			user: B2BOrgUser{Username: "carol", InviteStatus: InviteStatusRevoked},
			want: InviteStatusRevoked,
		},
		{
			name: "empty status with username derived as accepted",
			user: B2BOrgUser{Username: "dave", Email: "dave@example.com"},
			want: InviteStatusAccepted,
		},
		{
			name: "empty status without username derived as pending",
			user: B2BOrgUser{Email: "eve@example.com"},
			want: InviteStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.user.EffectiveStatus())
		})
	}
}

func TestB2BOrgSettings_ActiveWriterUsernames_LegacyNoStatus(t *testing.T) {
	// Admin backfill records may have no InviteStatus set.
	// Users with a username must still produce FGA tuples.
	settings := &B2BOrgSettings{
		Writers: []B2BOrgUser{
			{Username: "alice", Email: "alice@example.com"}, // no InviteStatus
			{Email: "bob@example.com"},                      // no username, no status → pending
		},
	}
	assert.Equal(t, []string{"alice"}, settings.ActiveWriterUsernames())
}

func TestB2BOrgSettings_ActiveWriterUsernames(t *testing.T) {
	tests := []struct {
		name     string
		settings *B2BOrgSettings
		want     []string
	}{
		{
			name: "returns only accepted entries with username",
			settings: &B2BOrgSettings{
				Writers: []B2BOrgUser{
					{Username: "alice", InviteStatus: InviteStatusAccepted},
					{Email: "bob@example.com", InviteStatus: InviteStatusPending},
					{Username: "carol", InviteStatus: InviteStatusRevoked},
					{Username: "dave", InviteStatus: InviteStatusExpired},
				},
			},
			want: []string{"alice"},
		},
		{
			name: "accepted entry without username is skipped",
			settings: &B2BOrgSettings{
				Writers: []B2BOrgUser{
					{Email: "nousername@example.com", InviteStatus: InviteStatusAccepted},
				},
			},
			want: nil,
		},
		{
			name:     "nil settings returns nil",
			settings: nil,
			want:     nil,
		},
		{
			name:     "empty writers returns nil",
			settings: &B2BOrgSettings{},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.settings.ActiveWriterUsernames()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestB2BOrgSettings_Tags_MemberTagDeduplication(t *testing.T) {
	// A user accepted as both writer and auditor must produce member: exactly once.
	settings := &B2BOrgSettings{
		UID:      "org-uid",
		Writers:  []B2BOrgUser{{Username: "charlie", InviteStatus: InviteStatusAccepted}},
		Auditors: []B2BOrgUser{{Username: "charlie", InviteStatus: InviteStatusAccepted}},
	}
	tags := settings.Tags()

	count := 0
	for _, tag := range tags {
		if tag == TagPrefixMember+"charlie" {
			count++
		}
	}
	assert.Equal(t, 1, count, "member:charlie must appear exactly once even when user is both writer and auditor")
	// Role-specific tags are still both emitted.
	assert.Contains(t, tags, TagPrefixWritersUsername+"charlie")
	assert.Contains(t, tags, TagPrefixAuditorsUsername+"charlie")
}

func TestB2BOrgSettings_ActiveAuditorUsernames(t *testing.T) {
	tests := []struct {
		name     string
		settings *B2BOrgSettings
		want     []string
	}{
		{
			name: "returns only accepted entries with username",
			settings: &B2BOrgSettings{
				Auditors: []B2BOrgUser{
					{Username: "viewer1", InviteStatus: InviteStatusAccepted},
					{Username: "viewer2", InviteStatus: InviteStatusAccepted},
					{Email: "pending@example.com", InviteStatus: InviteStatusPending},
				},
			},
			want: []string{"viewer1", "viewer2"},
		},
		{
			name:     "nil settings returns nil",
			settings: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.settings.ActiveAuditorUsernames()
			assert.Equal(t, tt.want, got)
		})
	}
}
