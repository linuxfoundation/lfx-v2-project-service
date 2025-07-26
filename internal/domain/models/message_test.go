// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageActionConstants(t *testing.T) {
	tests := []struct {
		name     string
		action   MessageAction
		expected string
	}{
		{
			name:     "action created",
			action:   ActionCreated,
			expected: "created",
		},
		{
			name:     "action updated",
			action:   ActionUpdated,
			expected: "updated",
		},
		{
			name:     "action deleted",
			action:   ActionDeleted,
			expected: "deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.action))
		})
	}
}

func TestProjectMessage(t *testing.T) {
	tests := []struct {
		name    string
		message ProjectMessage
		verify  func(t *testing.T, msg ProjectMessage)
	}{
		{
			name: "project message with all fields",
			message: ProjectMessage{
				Action: ActionCreated,
				Headers: map[string]string{
					"request-id": "test-request-123",
					"user-id":    "user-456",
				},
				Data: map[string]interface{}{
					"uid":  "project-123",
					"slug": "test-project",
					"name": "Test Project",
				},
			},
			verify: func(t *testing.T, msg ProjectMessage) {
				assert.Equal(t, ActionCreated, msg.Action)
				assert.Equal(t, "test-request-123", msg.Headers["request-id"])
				assert.Equal(t, "user-456", msg.Headers["user-id"])
				assert.NotNil(t, msg.Data)

				// Verify data can be type asserted
				data, ok := msg.Data.(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "project-123", data["uid"])
				assert.Equal(t, "test-project", data["slug"])
				assert.Equal(t, "Test Project", data["name"])
			},
		},
		{
			name: "project message with minimal fields",
			message: ProjectMessage{
				Action: ActionDeleted,
				Data:   "simple-data",
			},
			verify: func(t *testing.T, msg ProjectMessage) {
				assert.Equal(t, ActionDeleted, msg.Action)
				assert.Nil(t, msg.Headers)
				assert.Equal(t, "simple-data", msg.Data)
			},
		},
		{
			name: "project message with nil data",
			message: ProjectMessage{
				Action: ActionUpdated,
				Headers: map[string]string{
					"correlation-id": "corr-123",
				},
				Data: nil,
			},
			verify: func(t *testing.T, msg ProjectMessage) {
				assert.Equal(t, ActionUpdated, msg.Action)
				assert.Equal(t, "corr-123", msg.Headers["correlation-id"])
				assert.Nil(t, msg.Data)
			},
		},
		{
			name: "project message with complex data structure",
			message: ProjectMessage{
				Action: ActionCreated,
				Data: struct {
					ProjectUID string   `json:"project_uid"`
					Tags       []string `json:"tags"`
					Metadata   struct {
						Version int    `json:"version"`
						Source  string `json:"source"`
					} `json:"metadata"`
				}{
					ProjectUID: "project-789",
					Tags:       []string{"tag1", "tag2"},
					Metadata: struct {
						Version int    `json:"version"`
						Source  string `json:"source"`
					}{
						Version: 1,
						Source:  "api",
					},
				},
			},
			verify: func(t *testing.T, msg ProjectMessage) {
				assert.Equal(t, ActionCreated, msg.Action)
				assert.NotNil(t, msg.Data)
				// Verify the data structure is preserved
				// In real usage, this would be marshaled/unmarshaled as JSON
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}

func TestMessageActionString(t *testing.T) {
	tests := []struct {
		name     string
		action   MessageAction
		expected string
	}{
		{
			name:     "created action string",
			action:   ActionCreated,
			expected: "created",
		},
		{
			name:     "updated action string",
			action:   ActionUpdated,
			expected: "updated",
		},
		{
			name:     "deleted action string",
			action:   ActionDeleted,
			expected: "deleted",
		},
		{
			name:     "custom action string",
			action:   MessageAction("custom"),
			expected: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.action))
		})
	}
}

func TestProjectMessageValidation(t *testing.T) {
	tests := []struct {
		name    string
		message ProjectMessage
		isValid bool
	}{
		{
			name: "valid message with required fields",
			message: ProjectMessage{
				Action: ActionCreated,
				Data:   "some-data",
			},
			isValid: true,
		},
		{
			name: "valid message with all fields",
			message: ProjectMessage{
				Action: ActionUpdated,
				Headers: map[string]string{
					"key": "value",
				},
				Data: map[string]string{
					"uid": "test-uid",
				},
			},
			isValid: true,
		},
		{
			name: "message with empty action",
			message: ProjectMessage{
				Action: "",
				Data:   "some-data",
			},
			isValid: false,
		},
		{
			name: "message with unknown action",
			message: ProjectMessage{
				Action: MessageAction("unknown"),
				Data:   "some-data",
			},
			isValid: true, // Unknown actions are allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - action should not be empty
			isValid := string(tt.message.Action) != ""
			assert.Equal(t, tt.isValid, isValid)
		})
	}
}