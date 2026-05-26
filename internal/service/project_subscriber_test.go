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
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
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
		// ── No-op cases ──────────────────────────────────────────────────────────────
		{
			name: "no changes — no sends",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
				NewSettings: events.ProjectSettings{Writers: []events.UserInfo{alice}},
			},
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		// ── Addition cases (Phase 1 regression) ──────────────────────────────────────
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
			name: "two different LFID users added across roles — two emails sent",
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
			name: "same LFID user added to two roles simultaneously — one consolidated email",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "proj-1",
				OldSettings: events.ProjectSettings{},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
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
					return len(s.Writers) > 0 && s.Writers[0].Invite.UID == "invite-writer-uid"
				}), uint64(1)).Return(nil)
				r.On("CreateInviteMapping", mock.Anything, "invite-writer-uid", "proj-1").Return(nil)
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
					return len(s.Auditors) > 0 && s.Auditors[0].Invite.UID == "invite-auditor-uid"
				}), uint64(1)).Return(nil)
				r.On("CreateInviteMapping", mock.Anything, "invite-auditor-uid", "proj-1").Return(nil)
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
					return len(s.MeetingCoordinators) > 0 && s.MeetingCoordinators[0].Invite.UID == "invite-mc-uid"
				}), uint64(1)).Return(nil)
				r.On("CreateInviteMapping", mock.Anything, "invite-mc-uid", "proj-1").Return(nil)
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
		// ── Role-change cases (LFID) ──────────────────────────────────────────────────
		{
			name: "LFID user role swapped (Writer → Auditor) — role changed email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		{
			// Writer already implies View (Auditor); gaining Auditor on top is a no-op — no email.
			name: "LFID user gains Auditor on top of Writer — no email (Manage includes View)",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			name: "LFID user partially removed (Writer+Auditor → Auditor) — role changed email, not removal",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		// ── Removal cases ─────────────────────────────────────────────────────────────
		{
			name: "LFID user fully removed — role removed email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		{
			name: "non-LFID user removed — no sends",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{noLFIDWriter},
				},
				NewSettings: events.ProjectSettings{},
				Actor:       events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		// ── Non-LFID role-change cases ────────────────────────────────────────────────
		{
			name: "non-LFID user already auditor added as writer — invite only for new Writer role",
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
					if len(s.Writers) == 0 || s.Writers[0].Invite.UID != "invite-writer-uid" {
						return false
					}
					// Auditors entry must NOT have an invite set.
					if len(s.Auditors) == 0 || s.Auditors[0].Invite != nil {
						return false
					}
					return true
				}), uint64(1)).Return(nil)
				r.On("CreateInviteMapping", mock.Anything, "invite-writer-uid", "proj-1").Return(nil)
			},
		},
		// ── Error & edge-case cases ───────────────────────────────────────────────────
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
		// ── Manage+View permission hierarchy suppression ──────────────────────────────
		{
			// Meeting Coordinator does NOT supersede Auditor — gaining Auditor on top of MC is meaningful.
			name: "LFID Meeting Coordinator gains Auditor — role changed email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					MeetingCoordinators: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					MeetingCoordinators: []events.UserInfo{alice},
					Auditors:            []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		{
			// Auditor → Writer is a meaningful upgrade; email must be sent.
			name: "LFID Auditor gains Writer — role changed email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Auditors: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
		},
		{
			// Non-LFID Writer gains Auditor: no invite because Manage already includes View.
			name: "non-LFID Writer gains Auditor — no invite (Manage includes View)",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{noLFIDWriter},
				},
				NewSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{noLFIDWriter},
					Auditors: []events.UserInfo{{Email: noLFIDWriter.Email}},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			// Losing Auditor while keeping Writer is symmetric: Manage still includes View, no email.
			name: "LFID Writer+Auditor loses Auditor — no email (Manage includes View)",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Writers: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			// Writer supersedes Meeting Coordinator — gaining MC on top of Writer is a no-op.
			name: "LFID Writer gains Meeting Coordinator — no email (Writer supersedes MC)",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Writers:             []events.UserInfo{alice},
					MeetingCoordinators: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  0,
			wantInviteCount: 0,
		},
		{
			// Losing Writer while still having Auditor is a meaningful downgrade — email must be sent.
			name: "LFID Writer+Auditor loses Writer — role changed email sent",
			event: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "proj-1",
				OldSettings: events.ProjectSettings{
					Writers:  []events.UserInfo{alice},
					Auditors: []events.UserInfo{alice},
				},
				NewSettings: events.ProjectSettings{
					Auditors: []events.UserInfo{alice},
				},
				Actor: events.Actor{Name: "Admin"},
			},
			projectBase:     makeProjectBase("proj-1", "Demo", "demo"),
			wantEmailCount:  1,
			wantInviteCount: 0,
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
					InviteUID:      inviteReturnUID,
					RecipientEmail: "nonlfid@example.com",
					ExpiresAt:      time.Now().Add(30 * 24 * time.Hour),
				}, inviteReturnErr).Times(tt.wantInviteCount)
			}

			// Each successful invite write-back triggers a settings reindex.
			// The matcher verifies the envelope's Data is a ProjectSettings that has
			// the invite UID set on at least one user in the relevant role slice.
			wantIndexCount := 0
			if tt.inviteUID != "" && tt.msgBuilderErr == nil {
				wantIndexCount = tt.wantInviteCount
			}
			if wantIndexCount > 0 {
				wantInviteUID := tt.inviteUID
				mockMsg.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings",
					mock.MatchedBy(func(msg any) bool {
						env, ok := msg.(indexerTypes.IndexerMessageEnvelope)
						if !ok {
							return false
						}
						s, ok := env.Data.(models.ProjectSettings)
						if !ok {
							return false
						}
						for _, u := range append(append(s.Writers, s.Auditors...), s.MeetingCoordinators...) {
							if u.Invite != nil && u.Invite.UID == wantInviteUID {
								return true
							}
						}
						return false
					}), false).
					Return(nil).Times(wantIndexCount)
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

func TestDiffUserChanges(t *testing.T) {
	alice := events.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	bob := events.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noUsername := events.UserInfo{Email: "nouser@example.com", Name: "No Username"}
	empty := events.UserInfo{}

	tests := []struct {
		name         string
		old          events.ProjectSettings
		new          events.ProjectSettings
		wantLen      int
		wantContains []userChange
	}{
		{
			name: "no changes",
			old:  events.ProjectSettings{Writers: []events.UserInfo{alice}},
			new:  events.ProjectSettings{Writers: []events.UserInfo{alice}},
		},
		{
			name:    "writer added",
			old:     events.ProjectSettings{},
			new:     events.ProjectSettings{Writers: []events.UserInfo{alice}},
			wantLen: 1,
			wantContains: []userChange{
				{User: alice, NewRoles: []string{roleWriter}, Kind: changeAdded},
			},
		},
		{
			name:    "writer removed",
			old:     events.ProjectSettings{Writers: []events.UserInfo{alice}},
			new:     events.ProjectSettings{},
			wantLen: 1,
			wantContains: []userChange{
				{User: alice, OldRoles: []string{roleWriter}, Kind: changeRemoved},
			},
		},
		{
			name:    "writer role swapped to auditor — changed",
			old:     events.ProjectSettings{Writers: []events.UserInfo{alice}},
			new:     events.ProjectSettings{Auditors: []events.UserInfo{alice}},
			wantLen: 1,
			wantContains: []userChange{
				{User: alice, OldRoles: []string{roleWriter}, NewRoles: []string{roleAuditor}, Kind: changeChanged},
			},
		},
		{
			name: "auditor also gains writer — changed",
			old:  events.ProjectSettings{Auditors: []events.UserInfo{alice}},
			new: events.ProjectSettings{
				Writers:  []events.UserInfo{alice},
				Auditors: []events.UserInfo{alice},
			},
			wantLen: 1,
			wantContains: []userChange{
				{User: alice, OldRoles: []string{roleAuditor}, NewRoles: []string{roleWriter, roleAuditor}, Kind: changeChanged},
			},
		},
		{
			name: "partial removal (writer+auditor → auditor) — changed not removed",
			old: events.ProjectSettings{
				Writers:  []events.UserInfo{alice},
				Auditors: []events.UserInfo{alice},
			},
			new:     events.ProjectSettings{Auditors: []events.UserInfo{alice}},
			wantLen: 1,
			wantContains: []userChange{
				{User: alice, OldRoles: []string{roleWriter, roleAuditor}, NewRoles: []string{roleAuditor}, Kind: changeChanged},
			},
		},
		{
			name: "two different users each added in separate roles",
			old:  events.ProjectSettings{},
			new: events.ProjectSettings{
				Writers:  []events.UserInfo{alice},
				Auditors: []events.UserInfo{bob},
			},
			wantLen: 2,
		},
		{
			name:    "user with no username matched by email — added",
			old:     events.ProjectSettings{},
			new:     events.ProjectSettings{Writers: []events.UserInfo{noUsername}},
			wantLen: 1,
			wantContains: []userChange{
				{User: noUsername, NewRoles: []string{roleWriter}, Kind: changeAdded},
			},
		},
		{
			name: "user with neither username nor email is skipped",
			old:  events.ProjectSettings{},
			new:  events.ProjectSettings{Writers: []events.UserInfo{empty}},
		},
		{
			// Same person: email-only in old Writers, then gains a username in new Writers.
			// Role set is identical (Writer) → should be treated as no change.
			name:    "identity shape change (email-only → username+email) with same role — not a change",
			old:     events.ProjectSettings{Writers: []events.UserInfo{{Email: "alice@example.com"}}},
			new:     events.ProjectSettings{Writers: []events.UserInfo{{Username: "alice", Email: "alice@example.com"}}},
			wantLen: 0,
		},
		{
			// Same person: email-only Auditor in old, gains username and also becomes Writer.
			name: "identity shape change with role gain — classified as changed",
			old:  events.ProjectSettings{Auditors: []events.UserInfo{{Email: "alice@example.com"}}},
			new: events.ProjectSettings{
				Writers:  []events.UserInfo{{Username: "alice", Email: "alice@example.com"}},
				Auditors: []events.UserInfo{{Username: "alice", Email: "alice@example.com"}},
			},
			wantLen: 1,
			wantContains: []userChange{
				{
					User:     events.UserInfo{Username: "alice", Email: "alice@example.com"},
					OldRoles: []string{roleAuditor},
					NewRoles: []string{roleWriter, roleAuditor},
					Kind:     changeChanged,
				},
			},
		},
		{
			// Duplicate entries for alice in the new Writers slice. The per-user map
			// deduplicates so only one change is returned.
			name:    "duplicate LFID user in new snapshot — deduplicated",
			old:     events.ProjectSettings{},
			new:     events.ProjectSettings{Writers: []events.UserInfo{alice, alice}},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffUserChanges(tt.old, tt.new)
			assert.Len(t, got, tt.wantLen)
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}

func TestIsWriterSupersededNoOp(t *testing.T) {
	tests := []struct {
		name     string
		oldRoles []string
		newRoles []string
		want     bool
	}{
		{
			name:     "Writer gains Auditor — suppress",
			oldRoles: []string{roleWriter},
			newRoles: []string{roleWriter, roleAuditor},
			want:     true,
		},
		{
			name:     "Writer+Auditor loses Auditor — suppress",
			oldRoles: []string{roleWriter, roleAuditor},
			newRoles: []string{roleWriter},
			want:     true,
		},
		{
			name:     "Writer gains Meeting Coordinator — suppress",
			oldRoles: []string{roleWriter},
			newRoles: []string{roleWriter, roleMeetingCoordinator},
			want:     true,
		},
		{
			name:     "Writer+MC loses Meeting Coordinator — suppress",
			oldRoles: []string{roleWriter, roleMeetingCoordinator},
			newRoles: []string{roleWriter},
			want:     true,
		},
		{
			name:     "Writer gains both MC and Auditor — suppress",
			oldRoles: []string{roleWriter},
			newRoles: []string{roleWriter, roleMeetingCoordinator, roleAuditor},
			want:     true,
		},
		{
			// MC does NOT supersede Auditor: gaining Auditor while holding only MC is meaningful.
			name:     "Meeting Coordinator gains Auditor — not a no-op",
			oldRoles: []string{roleMeetingCoordinator},
			newRoles: []string{roleMeetingCoordinator, roleAuditor},
			want:     false,
		},
		{
			name:     "Auditor gains Writer — not a no-op",
			oldRoles: []string{roleAuditor},
			newRoles: []string{roleWriter, roleAuditor},
			want:     false,
		},
		{
			name:     "Writer swapped to Auditor — not a no-op",
			oldRoles: []string{roleWriter},
			newRoles: []string{roleAuditor},
			want:     false,
		},
		{
			name:     "identical roles — not a no-op (no change at all)",
			oldRoles: []string{roleWriter},
			newRoles: []string{roleWriter},
			want:     false,
		},
		{
			// Swapping MC for Auditor while keeping Writer is a meaningful subordinate change.
			name:     "Writer+MC swapped to Writer+Auditor — not a no-op",
			oldRoles: []string{roleWriter, roleMeetingCoordinator},
			newRoles: []string{roleWriter, roleAuditor},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWriterSupersededNoOp(tt.oldRoles, tt.newRoles)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRolesForDisplay(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  []string
	}{
		{name: "Writer → Manage", roles: []string{roleWriter}, want: []string{"Manage"}},
		{name: "Auditor → View", roles: []string{roleAuditor}, want: []string{"View"}},
		{name: "Meeting Coordinator → Meeting Coordinator", roles: []string{roleMeetingCoordinator}, want: []string{"Meeting Coordinator"}},
		{name: "Writer+Auditor → Manage only (Auditor dropped)", roles: []string{roleWriter, roleAuditor}, want: []string{"Manage"}},
		{name: "MC+Auditor → both shown (neither supersedes)", roles: []string{roleMeetingCoordinator, roleAuditor}, want: []string{"Meeting Coordinator", "View"}},
		{name: "Writer+MC → Manage only (MC dropped)", roles: []string{roleWriter, roleMeetingCoordinator}, want: []string{"Manage"}},
		{name: "Writer+MC+Auditor → Manage only (all subordinates dropped)", roles: []string{roleWriter, roleMeetingCoordinator, roleAuditor}, want: []string{"Manage"}},
		{name: "empty → empty", roles: nil, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rolesForDisplay(tt.roles)
			if tt.want == nil {
				tt.want = []string{}
			}
			if got == nil {
				got = []string{}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHandleInviteAccepted(t *testing.T) {
	const inviteUID = "inv-abc"
	const username = "newuser"
	const projectUID = "proj-1"

	makeSettings := func() *models.ProjectSettings {
		return &models.ProjectSettings{
			UID:     projectUID,
			Writers: []models.UserInfo{{Email: "writer@example.com", Invite: &models.InviteInfo{UID: inviteUID}}},
		}
	}

	indexMatcher := mock.MatchedBy(func(msg any) bool {
		env, ok := msg.(indexerTypes.IndexerMessageEnvelope)
		if !ok {
			return false
		}
		s, ok := env.Data.(models.ProjectSettings)
		if !ok {
			return false
		}
		for _, u := range s.Writers {
			if u.Username == username && u.Invite == nil {
				return true
			}
		}
		return false
	})

	tests := []struct {
		name      string
		payload   any
		setupRepo func(*domain.MockProjectRepository)
		setupMsg  func(*domain.MockMessageBuilder)
		wantErr   bool
	}{
		{
			name:    "malformed payload — returns nil without crashing",
			payload: []byte("not json"),
		},
		{
			name:    "missing invite_uid — discarded",
			payload: events.InviteAccepted{InviteUID: "", Username: username},
		},
		{
			name:    "missing username — discarded",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: ""},
		},
		{
			name:    "ErrInviteMappingNotFound — silently ignored (debug log, no warn)",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).
					Return("", domain.ErrInviteMappingNotFound)
			},
		},
		{
			name:    "KV error on GetProjectUIDByInviteUID — warn logged, nil returned",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).
					Return("", assert.AnError)
			},
		},
		{
			name:    "happy path — user promoted, mapping deleted, indexer called",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).Return(projectUID, nil)
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(makeSettings(), uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					return len(s.Writers) > 0 && s.Writers[0].Username == username && s.Writers[0].Invite == nil
				}), uint64(1)).Return(nil)
				r.On("DeleteInviteMapping", mock.Anything, inviteUID).Return(nil)
			},
			setupMsg: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings", indexMatcher, false).Return(nil)
			},
		},
		{
			name:    "ErrRevisionMismatch on UPDATE — succeeds on attempt 2",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).Return(projectUID, nil)
				// First GET + UPDATE fails with revision mismatch; second GET + UPDATE succeeds.
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).
					Return(makeSettings(), uint64(1), nil).Once()
				r.On("UpdateProjectSettings", mock.Anything, mock.Anything, uint64(1)).
					Return(domain.ErrRevisionMismatch).Once()
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).
					Return(makeSettings(), uint64(2), nil).Once()
				r.On("UpdateProjectSettings", mock.Anything, mock.MatchedBy(func(s *models.ProjectSettings) bool {
					return len(s.Writers) > 0 && s.Writers[0].Username == username && s.Writers[0].Invite == nil
				}), uint64(2)).Return(nil).Once()
				r.On("DeleteInviteMapping", mock.Anything, inviteUID).Return(nil)
			},
			setupMsg: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings", indexMatcher, false).Return(nil)
			},
		},
		{
			name:    "promoted == false (stale mapping) — mapping deleted, nil returned",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).Return(projectUID, nil)
				// Settings contain no entry with the invite UID.
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).
					Return(&models.ProjectSettings{UID: projectUID, Writers: []models.UserInfo{{Email: "other@example.com"}}}, uint64(1), nil)
				r.On("DeleteInviteMapping", mock.Anything, inviteUID).Return(nil)
			},
		},
		{
			name:    "mapping-delete failure after acceptance — warn logged, not fatal",
			payload: events.InviteAccepted{InviteUID: inviteUID, Username: username},
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectUIDByInviteUID", mock.Anything, inviteUID).Return(projectUID, nil)
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(makeSettings(), uint64(1), nil)
				r.On("UpdateProjectSettings", mock.Anything, mock.Anything, uint64(1)).Return(nil)
				r.On("DeleteInviteMapping", mock.Anything, inviteUID).Return(assert.AnError)
			},
			setupMsg: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings", indexMatcher, false).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domain.MockProjectRepository{}
			mockMsg := &domain.MockMessageBuilder{}
			if tt.setupRepo != nil {
				tt.setupRepo(mockRepo)
			}
			if tt.setupMsg != nil {
				tt.setupMsg(mockMsg)
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				MessageBuilder:    mockMsg,
			}

			var data []byte
			if raw, ok := tt.payload.([]byte); ok {
				data = raw
			} else {
				data = marshalEvent(t, tt.payload)
			}
			msg := domain.NewMockMessage(data, "")
			err := svc.HandleInviteAccepted(context.Background(), msg)
			assert.NoError(t, err)

			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}

func TestStoreInviteInfo(t *testing.T) {
	const projectUID = "proj-1"
	const inviteUID = "inv-xyz"
	const inviteEmail = "invite@example.com"
	const recipientEmail = "writer@example.com"

	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)

	makeSettings := func(rev uint64) (*models.ProjectSettings, uint64) {
		return &models.ProjectSettings{
			UID:     projectUID,
			Writers: []models.UserInfo{{Email: recipientEmail}},
		}, rev
	}

	inviteMatcher := mock.MatchedBy(func(s *models.ProjectSettings) bool {
		return len(s.Writers) > 0 && s.Writers[0].Invite != nil && s.Writers[0].Invite.UID == inviteUID
	})

	tests := []struct {
		name      string
		setupRepo func(*domain.MockProjectRepository)
		setupMsg  func(*domain.MockMessageBuilder)
		wantErr   bool
	}{
		{
			name: "happy path — invite info stamped and mapping created",
			setupRepo: func(r *domain.MockProjectRepository) {
				s, rev := makeSettings(1)
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(s, rev, nil)
				r.On("UpdateProjectSettings", mock.Anything, inviteMatcher, rev).Return(nil)
				r.On("CreateInviteMapping", mock.Anything, inviteUID, projectUID).Return(nil)
			},
			setupMsg: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings", mock.Anything, false).Return(nil)
			},
		},
		{
			name: "ErrRevisionMismatch twice then success on attempt 3",
			setupRepo: func(r *domain.MockProjectRepository) {
				s1, _ := makeSettings(1)
				s2, _ := makeSettings(2)
				s3, _ := makeSettings(3)
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(s1, uint64(1), nil).Once()
				r.On("UpdateProjectSettings", mock.Anything, mock.Anything, uint64(1)).Return(domain.ErrRevisionMismatch).Once()
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(s2, uint64(2), nil).Once()
				r.On("UpdateProjectSettings", mock.Anything, mock.Anything, uint64(2)).Return(domain.ErrRevisionMismatch).Once()
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(s3, uint64(3), nil).Once()
				r.On("UpdateProjectSettings", mock.Anything, inviteMatcher, uint64(3)).Return(nil).Once()
				r.On("CreateInviteMapping", mock.Anything, inviteUID, projectUID).Return(nil)
			},
			setupMsg: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, "lfx.index.project_settings", mock.Anything, false).Return(nil)
			},
		},
		{
			name: "user not found in role slice — no update, no error",
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(&models.ProjectSettings{
					UID:     projectUID,
					Writers: []models.UserInfo{{Email: "other@example.com"}},
				}, uint64(1), nil)
			},
		},
		{
			name: "KV read error — returned to caller",
			setupRepo: func(r *domain.MockProjectRepository) {
				r.On("GetProjectSettingsWithRevision", mock.Anything, projectUID).Return(nil, uint64(0), assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domain.MockProjectRepository{}
			mockMsg := &domain.MockMessageBuilder{}
			if tt.setupRepo != nil {
				tt.setupRepo(mockRepo)
			}
			if tt.setupMsg != nil {
				tt.setupMsg(mockMsg)
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				MessageBuilder:    mockMsg,
			}

			err := svc.storeInviteInfo(context.Background(), projectUID, roleWriter, recipientEmail, inviteUID, inviteEmail, expiresAt)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}

// Compile-time checks for imported API types.
var (
	_ emailapi.SendEmailRequest
	_ inviteapi.SendInviteRequest
)
