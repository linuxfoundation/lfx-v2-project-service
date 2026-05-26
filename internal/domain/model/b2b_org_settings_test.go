// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
