// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrgSettings_ActiveWriterUsernames(t *testing.T) {
	tests := []struct {
		name     string
		settings *OrgSettings
		want     []string
	}{
		{
			name: "returns only accepted entries with username",
			settings: &OrgSettings{
				Writers: []OrgUser{
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
			settings: &OrgSettings{
				Writers: []OrgUser{
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
			settings: &OrgSettings{},
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

func TestOrgSettings_ActiveAuditorUsernames(t *testing.T) {
	tests := []struct {
		name     string
		settings *OrgSettings
		want     []string
	}{
		{
			name: "returns only accepted entries with username",
			settings: &OrgSettings{
				Auditors: []OrgUser{
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
