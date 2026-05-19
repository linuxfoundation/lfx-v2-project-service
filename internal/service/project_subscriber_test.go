// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
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
	// Users WITH LFID (Username set) → direct notification email via email-service.
	alice := events.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	bob := events.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}

	// Users WITHOUT LFID (Username empty) → invite request via invite-service.
	noLFIDWriter := events.UserInfo{Email: "writer@example.com", Name: "No LFID Writer"}
	noLFIDAuditor := events.UserInfo{Email: "auditor@example.com", Name: "No LFID Auditor"}
	noLFIDMC := events.UserInfo{Email: "mc@example.com", Name: "No LFID MC"}

	// settingsWithWriter returns a minimal ProjectSettings containing a writer matching the
	// noLFIDWriter fixture — used to back the GetProjectSettingsWithRevision mock.
	settingsWithWriter := func() *models.ProjectSettings {
		return &models.ProjectSettings{
			UID:     "proj-1",
			Writers: []models.UserInfo{{Email: noLFIDWriter.Email}},
		}
	}
	settingsWithAuditor := func() *models.ProjectSettings {
		return &models.ProjectSettings{
			UID:      "proj-1",
			Auditors: []models.UserInfo{{Email: noLFIDAuditor.Email}},
		}
	}
	settingsWithMC := func() *models.ProjectSettings {
		return &models.ProjectSettings{
			UID:                 "proj-1",
			MeetingCoordinators: []models.UserInfo{{Email: noLFIDMC.Email}},
		}
	}

	tests := []struct {
		name              string
		event             events.ProjectSettingsUpdatedMessage
		projectBase       *models.ProjectBase
		projectBaseErr    error
		wantEmailCount    int
		wantInviteCount   int
		wantInviteRole    string // expected Role field in the SendInviteRequest payload
		inviteUID         string // invite UID returned by the mock (empty → no write-back)
		msgBuilderErr     error
		wantURLContains   string
		wantURLNotContain string
		setupRepoExtra    func(*domain.MockProjectRepository) // optional extra repo mock setup
	}{
		{
			name: "no additions — no sends",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
			},
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			name: "LFID writer added — direct email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Username: "admin", Name: "Admin User"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		{
			name: "two LFID users added across roles — two emails sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{bob},
				},
				Actor: events.Actor{Username: "admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  2,
			wantInviteCount: 0,
		},
		{
			name: "non-LFID writer added — invite request published and invite UID stored",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{noLFIDWriter}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 1,
			wantInviteRole:  string(inviteapi.InviteRoleManage),
			inviteUID:       "invite-writer-uid",
			setupRepoExtra: func(r *domain.MockProjectRepository) {
				r.On("GetProjectSettingsWithRevision", mock.Anything, "proj-1").
					Return(settingsWithWriter(), uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					return len(s.Writers) > 0 && s.Writers[0].InviteUID == "invite-writer-uid"
				}), uint64(1)).Return(nil)
			},
		},
		{
			name: "non-LFID auditor added — invite request published and invite UID stored",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Auditors: []events.UserInfo{noLFIDAuditor}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 1,
			wantInviteRole:  string(inviteapi.InviteRoleView),
			inviteUID:       "invite-auditor-uid",
			setupRepoExtra: func(r *domain.MockProjectRepository) {
				r.On("GetProjectSettingsWithRevision", mock.Anything, "proj-1").
					Return(settingsWithAuditor(), uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					return len(s.Auditors) > 0 && s.Auditors[0].InviteUID == "invite-auditor-uid"
				}), uint64(1)).Return(nil)
			},
		},
		{
			name: "non-LFID meeting coordinator added — invite request published with Manage role and UID stored",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{MeetingCoordinators: []events.UserInfo{noLFIDMC}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 1,
			wantInviteRole:  string(inviteapi.InviteRoleManage),
			inviteUID:       "invite-mc-uid",
			setupRepoExtra: func(r *domain.MockProjectRepository) {
				r.On("GetProjectSettingsWithRevision", mock.Anything, "proj-1").
					Return(settingsWithMC(), uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					return len(s.MeetingCoordinators) > 0 && s.MeetingCoordinators[0].InviteUID == "invite-mc-uid"
				}), uint64(1)).Return(nil)
			},
		},
		{
			name: "mixed LFID and non-LFID added — email for LFID, invite for non-LFID",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice, noLFIDWriter},
					Auditors: []events.UserInfo{noLFIDAuditor},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 2,
			// inviteUID is empty — no write-back expected (returned empty UID path)
		},
		{
			name: "send error on email — handler still returns nil",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Username: "admin"},
			},
			projectBase:    makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount: 1,
			msgBuilderErr:  assert.AnError,
		},
		{
			name: "send error on invite — handler still returns nil, no write-back",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{noLFIDWriter}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantInviteCount: 1,
			wantInviteRole:  string(inviteapi.InviteRoleManage),
			msgBuilderErr:   assert.AnError,
			// No setupRepoExtra — write-back must not happen when invite returns error.
		},
		{
			name: "user without email address — skipped entirely",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{{Username: "noemail", Name: "No Email"}}},
				Actor:       events.Actor{Username: "admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			name: "project load failure — no sends",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
			},
			projectBase:     nil,
			projectBaseErr:  assert.AnError,
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			// User is already an auditor but gets added as a writer.  Only the Writers
			// slice entry should receive the invite UID — the Auditors entry is untouched.
			name: "non-LFID user already auditor added as writer — invite UID stored only on writer entry",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Auditors: []events.UserInfo{{Email: noLFIDWriter.Email}},
				},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{noLFIDWriter},
					Auditors: []events.UserInfo{{Email: noLFIDWriter.Email}},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 1,
			wantInviteRole:  string(inviteapi.InviteRoleManage),
			inviteUID:       "invite-writer-uid",
			setupRepoExtra: func(r *domain.MockProjectRepository) {
				// Settings contains the user in both slices; only Writers entry should be stamped.
				r.On("GetProjectSettingsWithRevision", mock.Anything, "proj-1").
					Return(&models.ProjectSettings{
						UID:      "proj-1",
						Writers:  []models.UserInfo{{Email: noLFIDWriter.Email}},
						Auditors: []models.UserInfo{{Email: noLFIDWriter.Email}},
					}, uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					if len(s.Writers) == 0 || s.Writers[0].InviteUID != "invite-writer-uid" {
						return false
					}
					// Auditors entry must NOT have the invite UID set.
					if len(s.Auditors) == 0 || s.Auditors[0].InviteUID != "" {
						return false
					}
					return true
				}), uint64(1)).Return(nil)
			},
		},
		{
			name: "project with slug — URL includes slug query param",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "my-project"),
			wantEmailCount:  1,
			wantURLContains: "?project=my-project",
		},
		{
			name: "project without slug — fallback URL has no query param",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:       makeProjectBase("proj-1", "Demo", ""),
			wantEmailCount:    1,
			wantURLContains:   "project/overview",
			wantURLNotContain: "?project=",
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

			if tt.wantEmailCount > 0 {
				if tt.wantURLContains != "" || tt.wantURLNotContain != "" {
					wantContains := tt.wantURLContains
					wantNotContain := tt.wantURLNotContain
					mockMsg.On("SendEmailRequest", mock.Anything, mock.MatchedBy(func(req emailapi.SendEmailRequest) bool {
						if wantContains != "" && !strings.Contains(req.HTML, wantContains) {
							return false
						}
						if wantNotContain != "" && strings.Contains(req.HTML, wantNotContain) {
							return false
						}
						return true
					})).Return(tt.msgBuilderErr).Times(tt.wantEmailCount)
				} else {
					mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
						Return(tt.msgBuilderErr).Times(tt.wantEmailCount)
				}
			}

			if tt.wantInviteCount > 0 {
				wantRole := tt.wantInviteRole
				wantProjectUID := tt.event.ProjectUID
				inviteReturnUID := tt.inviteUID
				inviteReturnErr := tt.msgBuilderErr
				mockMsg.On("SendInviteRequest", mock.Anything, mock.MatchedBy(func(req inviteapi.SendInviteRequest) bool {
					return req.ResourceUID == wantProjectUID &&
						(wantRole == "" || req.Role == wantRole) &&
						req.RecipientEmail != "" &&
						req.ReturnURL != ""
				})).Return(domain.InviteResult{
					InviteUID: inviteReturnUID,
					ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
				}, inviteReturnErr).Times(tt.wantInviteCount)
			}

			if tt.setupRepoExtra != nil {
				tt.setupRepoExtra(mockRepo)
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				MessageBuilder:    mockMsg,
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
				},
			}

			msg := domain.NewMockMessage(marshalEvent(t, tt.event), "")
			err := svc.HandleProjectSettingsUpdated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockMsg.AssertNumberOfCalls(t, "SendInviteRequest", tt.wantInviteCount)
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

func TestMapRoleToInviteRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want string
	}{
		{"writer", roleWriter, string(inviteapi.InviteRoleManage)},
		{"auditor", roleAuditor, string(inviteapi.InviteRoleView)},
		{"meeting coordinator", roleMeetingCoordinator, string(inviteapi.InviteRoleManage)},
		{"unknown role", "Unknown", ""},
		{"empty role", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mapRoleToInviteRole(tt.role))
		})
	}
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
			wantContains: []roleAssignment{{User: bob, Role: roleAuditor}},
		},
		{
			name:         "meeting coordinator added",
			old:          events.ProjectSettings{},
			new:          events.ProjectSettings{MeetingCoordinators: []events.UserInfo{alice}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: alice, Role: roleMeetingCoordinator}},
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
			wantContains: []roleAssignment{{User: noUsername, Role: roleWriter}},
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
		{
			// Same person listed twice in new with different identity shapes (username+email,
			// then email-only). seenNew must index all keys so the second entry is recognised
			// as a duplicate and only one notification is sent.
			name: "same person twice in new with different identity shapes — only one addition",
			old:  events.ProjectSettings{},
			new: events.ProjectSettings{Writers: []events.UserInfo{
				{Username: "alice", Email: "alice@example.com"},
				{Email: "alice@example.com"},
			}},
			wantLen:      1,
			wantContains: []roleAssignment{{User: events.UserInfo{Username: "alice", Email: "alice@example.com"}, Role: roleWriter}},
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

// Compile-time checks for imported API types.
var (
	_ emailapi.SendEmailRequest
	_ inviteapi.SendInviteRequest
)
