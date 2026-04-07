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
		message GenericFGAMessage
		verify  func(t *testing.T, msg GenericFGAMessage)
	}{
		{
			name: "generic FGA message for update_access",
			message: GenericFGAMessage{
				ObjectType: "project",
				Operation:  "update_access",
				Data: UpdateAccessData{
					UID:    "project-123",
					Public: true,
					Relations: map[string][]string{
						"writer":              {"user1", "user2"},
						"auditor":             {"auditor1"},
						"meeting_coordinator": {"coordinator1"},
					},
					References: map[string][]string{
						"parent": {"project:parent-456"},
					},
				},
			},
			verify: func(t *testing.T, msg GenericFGAMessage) {
				assert.Equal(t, "project", msg.ObjectType)
				assert.Equal(t, "update_access", msg.Operation)

				data, ok := msg.Data.(UpdateAccessData)
				assert.True(t, ok)
				assert.Equal(t, "project-123", data.UID)
				assert.True(t, data.Public)
				assert.Len(t, data.Relations, 3)
				assert.Len(t, data.Relations["writer"], 2)
				assert.Len(t, data.Relations["auditor"], 1)
				assert.Len(t, data.Relations["meeting_coordinator"], 1)
				assert.Len(t, data.References, 1)
				assert.Len(t, data.References["parent"], 1)
				assert.Equal(t, "project:parent-456", data.References["parent"][0])
			},
		},
		{
			name: "generic FGA message for delete_access",
			message: GenericFGAMessage{
				ObjectType: "project",
				Operation:  "delete_access",
				Data: DeleteAccessData{
					UID: "project-789",
				},
			},
			verify: func(t *testing.T, msg GenericFGAMessage) {
				assert.Equal(t, "project", msg.ObjectType)
				assert.Equal(t, "delete_access", msg.Operation)

				data, ok := msg.Data.(DeleteAccessData)
				assert.True(t, ok)
				assert.Equal(t, "project-789", data.UID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}
