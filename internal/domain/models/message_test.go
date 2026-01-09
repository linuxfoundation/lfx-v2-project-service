// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectAccessMessage(t *testing.T) {
	tests := []struct {
		name    string
		message ProjectAccessMessage
		verify  func(t *testing.T, msg ProjectAccessMessage)
	}{
		{
			name: "project access message with all fields",
			message: ProjectAccessMessage{
				Data: ProjectAccessData{
					UID:                 "access-123",
					Public:              true,
					ParentUID:           "parent-456",
					Writers:             []string{"user1", "user2"},
					Auditors:            []string{"auditor1"},
					MeetingCoordinators: []string{"coordinator1"},
				},
			},
			verify: func(t *testing.T, msg ProjectAccessMessage) {
				assert.Equal(t, "access-123", msg.Data.UID)
				assert.True(t, msg.Data.Public)
				assert.Equal(t, "parent-456", msg.Data.ParentUID)
				assert.Len(t, msg.Data.Writers, 2)
				assert.Len(t, msg.Data.Auditors, 1)
				assert.Len(t, msg.Data.MeetingCoordinators, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}
