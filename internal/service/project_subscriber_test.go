// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"testing"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

func marshalEvent(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func makeProjectBase(uid, name, slug string) *models.ProjectBase {
	return &models.ProjectBase{UID: uid, Name: name, Slug: slug}
}

func TestHandleProjectSettingsUpdated(t *testing.T) {
	alice := events.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	bob := events.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}

	tests := []struct {
		name           string
		event          events.ProjectSettingsUpdatedMessage
		projectBase    *models.ProjectBase
		projectBaseErr error
		wantSendCount  int
		msgBuilderErr  error
	}{
		{
			name: "no additions — no emails sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
			},
			// projectBase intentionally nil: handler returns before GetProjectBase when no additions found
			wantSendCount: 0,
		},
		{
			name: "one writer added — one email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Username: "admin", Name: "Admin User"},
			},
			projectBase:   makeProjectBase("proj-1", "Demo", "demo"),
			wantSendCount: 1,
		},
		{
			name: "two users added across roles — two emails sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{bob},
				},
				Actor: events.Actor{Username: "admin"},
			},
			projectBase:   makeProjectBase("proj-1", "Demo", "demo"),
			wantSendCount: 2,
		},
		{
			name: "send error on one — other still attempted",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Username: "admin"},
			},
			projectBase:   makeProjectBase("proj-1", "Demo", "demo"),
			wantSendCount: 1,
			msgBuilderErr: assert.AnError,
		},
		{
			name: "user without email address — skipped, no send",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{{Username: "noemail", Name: "No Email"}}},
				Actor:       events.Actor{Username: "admin"},
			},
			projectBase:   makeProjectBase("proj-1", "Demo", "demo"),
			wantSendCount: 0,
		},
		{
			name: "project load failure — no email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
			},
			projectBase:    nil,
			projectBaseErr: assert.AnError,
			wantSendCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domain.MockProjectRepository{}
			mockMsg := &domain.MockMessageBuilder{}

			if tt.projectBase != nil || tt.projectBaseErr != nil {
				mockRepo.On("GetProjectBase", mock.Anything, tt.event.ProjectUID).
					Return(tt.projectBase, tt.projectBaseErr)
			}

			if tt.wantSendCount > 0 {
				mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
					Return(tt.msgBuilderErr).Times(tt.wantSendCount)
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				MessageBuilder:    mockMsg,
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://dev.app.lfx.dev",
				},
			}

			msg := domain.NewMockMessage(marshalEvent(t, tt.event), "")
			err := svc.HandleProjectSettingsUpdated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantSendCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil", func(t *testing.T) {
		svc := &ProjectsService{}
		msg := domain.NewMockMessage([]byte("not json"), "")
		err := svc.HandleProjectSettingsUpdated(context.Background(), msg)
		assert.NoError(t, err)
	})
}

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
		{
			name:         "duplicate entries in new — only one addition returned",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{Writers: []events.UserInfo{alice, alice}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: alice, Role: "Writer"}},
		},
		{
			// Same person appears email-only in old, then gains a username in new.
			// The multi-key lookup must recognise the shared email and not treat them as
			// a new addition.
			name:    "identity shape change — email-only in old, username+email in new — not a new addition",
			old:     events.ProjectSettings{Writers: []events.UserInfo{{Email: "alice@example.com"}}},
			new:     events.ProjectSettings{Writers: []events.UserInfo{{Username: "alice", Email: "alice@example.com"}}},
			wantLen: 0,
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

// Compile-time check: emailapi.SendEmailRequest is used to ensure the type alias is correct.
var _ emailapi.SendEmailRequest
