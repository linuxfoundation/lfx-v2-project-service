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

func TestProjectIndexerMessage(t *testing.T) {
	tests := []struct {
		name    string
		message ProjectIndexerMessage
		verify  func(t *testing.T, msg ProjectIndexerMessage)
	}{
		{
			name: "project indexer message with all fields",
			message: ProjectIndexerMessage{
				Action: ActionCreated,
				Data: ProjectBase{
					UID:  "project-123",
					Slug: "test-project",
					Name: "Test Project",
				},
				Tags: []string{"project-123", "test-project", "Test Project"},
			},
			verify: func(t *testing.T, msg ProjectIndexerMessage) {
				assert.Equal(t, ActionCreated, msg.Action)
				assert.Equal(t, "project-123", msg.Data.UID)
				assert.Equal(t, "test-project", msg.Data.Slug)
				assert.Equal(t, "Test Project", msg.Data.Name)
				assert.Len(t, msg.Tags, 3)
			},
		},
		{
			name: "project indexer message with minimal fields",
			message: ProjectIndexerMessage{
				Action: ActionDeleted,
				Data: ProjectBase{
					UID: "project-456",
				},
			},
			verify: func(t *testing.T, msg ProjectIndexerMessage) {
				assert.Equal(t, ActionDeleted, msg.Action)
				assert.Equal(t, "project-456", msg.Data.UID)
				assert.Empty(t, msg.Tags)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}

func TestProjectSettingsIndexerMessage(t *testing.T) {
	tests := []struct {
		name    string
		message ProjectSettingsIndexerMessage
		verify  func(t *testing.T, msg ProjectSettingsIndexerMessage)
	}{
		{
			name: "project settings indexer message with all fields",
			message: ProjectSettingsIndexerMessage{
				Action: ActionUpdated,
				Data: ProjectSettings{
					UID:              "settings-123",
					MissionStatement: "Our mission",
				},
				Tags: []string{"settings-123", "Our mission"},
			},
			verify: func(t *testing.T, msg ProjectSettingsIndexerMessage) {
				assert.Equal(t, ActionUpdated, msg.Action)
				assert.Equal(t, "settings-123", msg.Data.UID)
				assert.Equal(t, "Our mission", msg.Data.MissionStatement)
				assert.Len(t, msg.Tags, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}

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

func TestIndexerMessageEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		message IndexerMessageEnvelope
		verify  func(t *testing.T, msg IndexerMessageEnvelope)
	}{
		{
			name: "indexer message envelope with all fields",
			message: IndexerMessageEnvelope{
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
				Tags: []string{"project-123", "test-project"},
			},
			verify: func(t *testing.T, msg IndexerMessageEnvelope) {
				assert.Equal(t, ActionCreated, msg.Action)
				assert.Equal(t, "test-request-123", msg.Headers["request-id"])
				assert.Equal(t, "user-456", msg.Headers["user-id"])
				assert.NotNil(t, msg.Data)
				assert.Len(t, msg.Tags, 2)

				// Verify data can be type asserted
				data, ok := msg.Data.(map[string]interface{})
				assert.True(t, ok)
				assert.Equal(t, "project-123", data["uid"])
				assert.Equal(t, "test-project", data["slug"])
				assert.Equal(t, "Test Project", data["name"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.verify(t, tt.message)
		})
	}
}