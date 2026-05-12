// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

func TestDiffNewMembers(t *testing.T) {
	alice := events.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	bob := events.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noUsername := events.UserInfo{Email: "nouser@example.com", Name: "No Username"}
	empty := events.UserInfo{}

	tests := []struct {
		name         string
		old          events.ProjectSettings
		new          events.ProjectSettings
		wantLen      int
		wantContains []roleAssignment
	}{
		{
			name: "no changes",
			old:  events.ProjectSettings{Writers: []events.UserInfo{alice}},
			new:  events.ProjectSettings{Writers: []events.UserInfo{alice}},
		},
		{
			name:         "writer added",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{Writers: []events.UserInfo{alice}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: alice, Role: "Writer"}},
		},
		{
			name:         "auditor added",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{Auditors: []events.UserInfo{bob}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: bob, Role: "Auditor"}},
		},
		{
			name:         "meeting coordinator added",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{MeetingCoordinators: []events.UserInfo{alice}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: alice, Role: "Meeting Coordinator"}},
		},
		{
			name: "multiple roles added",
			old:  events.ProjectSettings{},
			new: events.ProjectSettings{
				Writers:  []events.UserInfo{alice},
				Auditors: []events.UserInfo{bob},
			},
			wantLen: 2,
		},
		{
			name: "removal only — no additions",
			old:  events.ProjectSettings{Writers: []events.UserInfo{alice, bob}},
			new:  events.ProjectSettings{Writers: []events.UserInfo{alice}},
		},
		{
			name:         "user with no username matched by email",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{Writers: []events.UserInfo{noUsername}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: noUsername, Role: "Writer"}},
		},
		{
			name: "user with neither username nor email is skipped",
			old:  events.ProjectSettings{},
			new:  events.ProjectSettings{Writers: []events.UserInfo{empty}},
		},
		{
			name: "existing user with no username skipped in old set",
			old:  events.ProjectSettings{Writers: []events.UserInfo{noUsername}},
			new:  events.ProjectSettings{Writers: []events.UserInfo{noUsername}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffNewMembers(tt.old, tt.new)
			assert.Len(t, got, tt.wantLen)
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}
