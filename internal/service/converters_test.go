// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"
	"time"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToDBProjectBase(t *testing.T) {
	tests := []struct {
		name     string
		input    *projsvc.ProjectBase
		expected *models.ProjectBase
		wantErr  bool
	}{
		{
			name: "valid project base conversion",
			input: &projsvc.ProjectBase{
				UID:         misc.StringPtr("test-uid"),
				Slug:        misc.StringPtr("test-slug"),
				Name:        misc.StringPtr("Test Project"),
				Description: misc.StringPtr("Test Description"),
				Public:      misc.BoolPtr(true),
				ParentUID:   misc.StringPtr("parent-uid"),
			},
			expected: &models.ProjectBase{
				UID:         "test-uid",
				Slug:        "test-slug",
				Name:        "Test Project",
				Description: "Test Description",
				Public:      true,
				ParentUID:   "parent-uid",
			},
			wantErr: false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: &models.ProjectBase{},
			wantErr:  false,
		},
		{
			name: "with dates",
			input: &projsvc.ProjectBase{
				UID:                   misc.StringPtr("test-uid"),
				FormationDate:         misc.StringPtr("2020-01-15"),
				EntityDissolutionDate: misc.StringPtr("2023-12-31"),
			},
			expected: &models.ProjectBase{
				UID: "test-uid",
			},
			wantErr: false,
		},
		{
			name: "invalid formation date",
			input: &projsvc.ProjectBase{
				UID:           misc.StringPtr("test-uid"),
				FormationDate: misc.StringPtr("invalid-date"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToDBProjectBase(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.expected != nil {
				assert.Equal(t, tt.expected.UID, result.UID)
				assert.Equal(t, tt.expected.Slug, result.Slug)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, tt.expected.Description, result.Description)
				assert.Equal(t, tt.expected.Public, result.Public)
				assert.Equal(t, tt.expected.ParentUID, result.ParentUID)
			}
		})
	}
}

func TestConvertToDBProjectSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    *projsvc.ProjectSettings
		existing *models.ProjectSettings // simulates the stored record on a PUT
		expected *models.ProjectSettings
		wantErr  bool
	}{
		{
			name: "valid project settings conversion",
			input: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				MissionStatement: misc.StringPtr("Test Mission"),
				Writers: []*projsvc.UserInfo{
					createTestAPIUserInfo("writer1", "Writer One", "writer1@example.com", ""),
					createTestAPIUserInfo("writer2", "Writer Two", "writer2@example.com", ""),
				},
				Auditors: []*projsvc.UserInfo{
					createTestAPIUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
				MeetingCoordinators: []*projsvc.UserInfo{
					createTestAPIUserInfo("coordinator1", "Coordinator One", "coordinator1@example.com", ""),
				},
			},
			expected: &models.ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Test Mission",
				Writers: []models.UserInfo{
					createTestUserInfo("writer1", "Writer One", "writer1@example.com", ""),
					createTestUserInfo("writer2", "Writer Two", "writer2@example.com", ""),
				},
				Auditors: []models.UserInfo{
					createTestUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
				MeetingCoordinators: []models.UserInfo{
					createTestUserInfo("coordinator1", "Coordinator One", "coordinator1@example.com", ""),
				},
			},
			wantErr: false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: &models.ProjectSettings{},
			wantErr:  false,
		},
		{
			name: "with announcement date",
			input: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				AnnouncementDate: misc.StringPtr("2023-06-01"),
			},
			expected: &models.ProjectSettings{
				UID: "test-uid",
			},
			wantErr: false,
		},
		{
			name: "invalid announcement date",
			input: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				AnnouncementDate: misc.StringPtr("invalid-date"),
			},
			wantErr: true,
		},
		{
			name: "with program_manager and opportunity_owner",
			input: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				ProgramManager:   createTestAPIUserInfo("pm1", "PM One", "pm1@example.com", ""),
				OpportunityOwner: createTestAPIUserInfo("oo1", "OO One", "oo1@example.com", ""),
			},
			expected: &models.ProjectSettings{
				UID:              "test-uid",
				ProgramManager:   &models.UserInfo{Username: "pm1", Name: "PM One", Email: "pm1@example.com"},
				OpportunityOwner: &models.UserInfo{Username: "oo1", Name: "OO One", Email: "oo1@example.com"},
			},
			wantErr: false,
		},
		{
			name: "invite preserved when user still in list (PUT does not wipe invite)",
			existing: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{Email: "w@example.com", Name: "Old Name", Invite: &models.InviteInfo{UID: "inv-1", Email: "w@example.com"}},
				},
			},
			input: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
				Writers: []*projsvc.UserInfo{
					{Name: misc.StringPtr("New Name"), Email: misc.StringPtr("w@example.com"), Username: misc.StringPtr(""), Avatar: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{Name: "New Name", Email: "w@example.com", Invite: &models.InviteInfo{UID: "inv-1", Email: "w@example.com"}},
				},
			},
		},
		{
			name: "empty API username preserves stored LFID",
			existing: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{Email: "gone@example.com", Username: "stored-lfid", Name: "Old Name"},
				},
			},
			input: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
				Writers: []*projsvc.UserInfo{
					{Name: misc.StringPtr("Old Name"), Email: misc.StringPtr("gone@example.com"), Username: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{Name: "Old Name", Email: "gone@example.com", Username: "stored-lfid"},
				},
			},
		},
		{
			name: "empty API executive director username preserves stored LFID",
			existing: &models.ProjectSettings{
				UID: "test-uid",
				ExecutiveDirector: &models.UserInfo{
					Email:    "ed@example.com",
					Username: "stored-ed",
					Name:     "Old ED",
				},
			},
			input: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
				ExecutiveDirector: &projsvc.UserInfo{
					Name:     misc.StringPtr("Old ED"),
					Email:    misc.StringPtr("ed@example.com"),
					Username: misc.StringPtr(""),
				},
			},
			expected: &models.ProjectSettings{
				UID: "test-uid",
				ExecutiveDirector: &models.UserInfo{
					Name:     "Old ED",
					Email:    "ed@example.com",
					Username: "stored-ed",
				},
			},
		},
		{
			name: "invite gone when user removed from list",
			existing: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{Email: "gone@example.com", Invite: &models.InviteInfo{UID: "inv-2", Email: "gone@example.com"}},
				},
			},
			input: &projsvc.ProjectSettings{
				UID:     misc.StringPtr("test-uid"),
				Writers: []*projsvc.UserInfo{}, // user removed
			},
			expected: &models.ProjectSettings{
				UID:     "test-uid",
				Writers: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToDBProjectSettings(tt.input, tt.existing)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.expected != nil {
				assert.Equal(t, tt.expected.UID, result.UID)
				assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
				assert.Equal(t, tt.expected.Writers, result.Writers)
				assert.Equal(t, tt.expected.Auditors, result.Auditors)
				assert.Equal(t, tt.expected.MeetingCoordinators, result.MeetingCoordinators)
				assert.Equal(t, tt.expected.ExecutiveDirector, result.ExecutiveDirector)
				assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
				assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
			}
		})
	}
}

func TestConvertToProjectFull(t *testing.T) {
	now := time.Now()
	announcementDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	formationDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		base     *models.ProjectBase
		settings *models.ProjectSettings
		expected *projsvc.ProjectFull
	}{
		{
			name: "complete project with settings",
			base: &models.ProjectBase{
				UID:           "test-uid",
				Slug:          "test-slug",
				Name:          "Test Project",
				Description:   "Test Description",
				Public:        true,
				ParentUID:     "parent-uid",
				FormationDate: &formationDate,
				CreatedAt:     &now,
				UpdatedAt:     &now,
			},
			settings: &models.ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Test Mission",
				AnnouncementDate: &announcementDate,
				Writers: []models.UserInfo{
					createTestUserInfo("writer1", "Writer One", "writer1@example.com", ""),
				},
				Auditors: []models.UserInfo{
					createTestUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
			},
			expected: &projsvc.ProjectFull{
				UID:              misc.StringPtr("test-uid"),
				Slug:             misc.StringPtr("test-slug"),
				Name:             misc.StringPtr("Test Project"),
				Description:      misc.StringPtr("Test Description"),
				Public:           misc.BoolPtr(true),
				ParentUID:        misc.StringPtr("parent-uid"),
				MissionStatement: misc.StringPtr("Test Mission"),
				Writers: []*projsvc.UserInfo{
					createTestAPIUserInfo("writer1", "Writer One", "writer1@example.com", ""),
				},
				Auditors: []*projsvc.UserInfo{
					createTestAPIUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
			},
		},
		{
			name: "with program_manager and opportunity_owner",
			base: &models.ProjectBase{
				UID:  "test-uid",
				Slug: "test-slug",
			},
			settings: &models.ProjectSettings{
				UID:              "test-uid",
				ProgramManager:   &models.UserInfo{Username: "pm1", Name: "PM One", Email: "pm1@example.com"},
				OpportunityOwner: &models.UserInfo{Username: "oo1", Name: "OO One", Email: "oo1@example.com"},
			},
			expected: &projsvc.ProjectFull{
				UID:              misc.StringPtr("test-uid"),
				Slug:             misc.StringPtr("test-slug"),
				Public:           misc.BoolPtr(false),
				AutojoinEnabled:  misc.BoolPtr(false),
				ProgramManager:   createTestAPIUserInfo("pm1", "PM One", "pm1@example.com", ""),
				OpportunityOwner: createTestAPIUserInfo("oo1", "OO One", "oo1@example.com", ""),
			},
		},
		{
			name:     "nil base",
			base:     nil,
			settings: &models.ProjectSettings{},
			expected: nil,
		},
		{
			name: "base without settings",
			base: &models.ProjectBase{
				UID:    "test-uid",
				Slug:   "test-slug",
				Public: false,
			},
			settings: nil,
			expected: &projsvc.ProjectFull{
				UID:             misc.StringPtr("test-uid"),
				Slug:            misc.StringPtr("test-slug"),
				Public:          misc.BoolPtr(false),
				AutojoinEnabled: misc.BoolPtr(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToProjectFull(tt.base, tt.settings)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected.UID, result.UID)
				assert.Equal(t, tt.expected.Slug, result.Slug)
				assert.Equal(t, tt.expected.Public, result.Public)
				if tt.expected.Name != nil {
					assert.Equal(t, tt.expected.Name, result.Name)
				}
				if tt.expected.MissionStatement != nil {
					assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
				}
				assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
				assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
			}
		})
	}
}

func TestConvertToServiceProjectBase(t *testing.T) {
	now := time.Now()
	formationDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *models.ProjectBase
		expected *projsvc.ProjectBase
	}{
		{
			name: "complete project base",
			input: &models.ProjectBase{
				UID:             "test-uid",
				Slug:            "test-slug",
				Name:            "Test Project",
				Description:     "Test Description",
				Public:          true,
				ParentUID:       "parent-uid",
				Stage:           "sandbox",
				Category:        "infrastructure",
				FormationDate:   &formationDate,
				AutojoinEnabled: true,
				CreatedAt:       &now,
				UpdatedAt:       &now,
			},
			expected: &projsvc.ProjectBase{
				UID:             misc.StringPtr("test-uid"),
				Slug:            misc.StringPtr("test-slug"),
				Name:            misc.StringPtr("Test Project"),
				Description:     misc.StringPtr("Test Description"),
				Public:          misc.BoolPtr(true),
				ParentUID:       misc.StringPtr("parent-uid"),
				Stage:           misc.StringPtr("sandbox"),
				Category:        misc.StringPtr("infrastructure"),
				AutojoinEnabled: misc.BoolPtr(true),
			},
		},
		{
			name: "minimal project base",
			input: &models.ProjectBase{
				UID:    "test-uid",
				Slug:   "test-slug",
				Public: false,
			},
			expected: &projsvc.ProjectBase{
				UID:             misc.StringPtr("test-uid"),
				Slug:            misc.StringPtr("test-slug"),
				Public:          misc.BoolPtr(false),
				AutojoinEnabled: misc.BoolPtr(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToServiceProjectBase(tt.input)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.Slug, result.Slug)
			assert.Equal(t, tt.expected.Public, result.Public)
			assert.Equal(t, tt.expected.AutojoinEnabled, result.AutojoinEnabled)
			if tt.expected.Name != nil {
				assert.Equal(t, tt.expected.Name, result.Name)
			}
			if tt.expected.Stage != nil {
				assert.Equal(t, tt.expected.Stage, result.Stage)
			}
		})
	}
}

func TestConvertToServiceProjectSettings(t *testing.T) {
	now := time.Now()
	announcementDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *models.ProjectSettings
		expected *projsvc.ProjectSettings
	}{
		{
			name: "complete project settings",
			input: &models.ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Test Mission",
				AnnouncementDate: &announcementDate,
				Writers: []models.UserInfo{
					createTestUserInfo("writer1", "Writer One", "writer1@example.com", ""),
					createTestUserInfo("writer2", "Writer Two", "writer2@example.com", ""),
				},
				Auditors: []models.UserInfo{
					createTestUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
				MeetingCoordinators: []models.UserInfo{
					createTestUserInfo("coordinator1", "Coordinator One", "coordinator1@example.com", ""),
				},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			expected: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				MissionStatement: misc.StringPtr("Test Mission"),
				Writers: []*projsvc.UserInfo{
					createTestAPIUserInfo("writer1", "Writer One", "writer1@example.com", ""),
					createTestAPIUserInfo("writer2", "Writer Two", "writer2@example.com", ""),
				},
				Auditors: []*projsvc.UserInfo{
					createTestAPIUserInfo("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
				MeetingCoordinators: []*projsvc.UserInfo{
					createTestAPIUserInfo("coordinator1", "Coordinator One", "coordinator1@example.com", ""),
				},
			},
		},
		{
			name: "minimal project settings",
			input: &models.ProjectSettings{
				UID: "test-uid",
			},
			expected: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
			},
		},
		{
			name: "with program_manager and opportunity_owner",
			input: &models.ProjectSettings{
				UID:              "test-uid",
				ProgramManager:   &models.UserInfo{Username: "pm1", Name: "PM One", Email: "pm1@example.com"},
				OpportunityOwner: &models.UserInfo{Username: "oo1", Name: "OO One", Email: "oo1@example.com"},
			},
			expected: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				ProgramManager:   createTestAPIUserInfo("pm1", "PM One", "pm1@example.com", ""),
				OpportunityOwner: createTestAPIUserInfo("oo1", "OO One", "oo1@example.com", ""),
			},
		},
		{
			name: "writer with pending invite is included in response",
			input: &models.ProjectSettings{
				UID: "test-uid",
				Writers: []models.UserInfo{
					{
						Email: "nolfid@example.com",
						Name:  "No LFID User",
						Invite: &models.InviteInfo{
							UID:   "invite-uid-123",
							Email: "nolfid@example.com",
						},
					},
				},
			},
			expected: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
				Writers: []*projsvc.UserInfo{
					{
						Name:     misc.StringPtr("No LFID User"),
						Email:    misc.StringPtr("nolfid@example.com"),
						Username: misc.StringPtr(""),
						Avatar:   misc.StringPtr(""),
						Invite: &projsvc.InviteInfo{
							UID:   misc.StringPtr("invite-uid-123"),
							Email: misc.StringPtr("nolfid@example.com"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToServiceProjectSettings(tt.input)
			assert.Equal(t, tt.expected.UID, result.UID)
			if tt.expected.MissionStatement != nil {
				assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			}
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)
			assert.Equal(t, tt.expected.MeetingCoordinators, result.MeetingCoordinators)
			assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
			assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
		})
	}
}

func TestDomainSettingsToEvent(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		input    *models.ProjectSettings
		expected events.ProjectSettings
	}{
		{
			name:     "nil input returns zero value",
			input:    nil,
			expected: events.ProjectSettings{},
		},
		{
			name: "full settings mapped correctly",
			input: &models.ProjectSettings{
				UID:              "uid-1",
				MissionStatement: "test mission",
				AnnouncementDate: &now,
				Auditors: []models.UserInfo{
					{Name: "A", Email: "a@example.com", Username: "auser", Avatar: "a.png"},
				},
				Writers: []models.UserInfo{
					{Name: "W", Email: "w@example.com", Username: "wuser", Avatar: "w.png"},
				},
				MeetingCoordinators: []models.UserInfo{
					{Name: "M", Email: "m@example.com", Username: "muser", Avatar: "m.png"},
				},
				ExecutiveDirector: &models.UserInfo{Name: "ED", Email: "ed@example.com", Username: "eduser", Avatar: "ed.png"},
				MentorshipProgramAdmins: []models.UserInfo{
					{Name: "MA", Email: "ma@example.com", Username: "mauser", Avatar: "ma.png"},
				},
				ProgramManager:   &models.UserInfo{Name: "PM", Email: "pm@example.com", Username: "pmuser", Avatar: "pm.png"},
				OpportunityOwner: &models.UserInfo{Name: "OO", Email: "oo@example.com", Username: "oouser", Avatar: "oo.png"},
				CreatedAt:        &now,
				UpdatedAt:        &now,
			},
			expected: events.ProjectSettings{
				UID:              "uid-1",
				MissionStatement: "test mission",
				AnnouncementDate: &now,
				Auditors:         []events.UserInfo{{Name: "A", Email: "a@example.com", Username: "auser", Avatar: "a.png"}},
				Writers:          []events.UserInfo{{Name: "W", Email: "w@example.com", Username: "wuser", Avatar: "w.png"}},
				MeetingCoordinators: []events.UserInfo{
					{Name: "M", Email: "m@example.com", Username: "muser", Avatar: "m.png"},
				},
				ExecutiveDirector: &events.UserInfo{Name: "ED", Email: "ed@example.com", Username: "eduser", Avatar: "ed.png"},
				MentorshipProgramAdmins: []events.UserInfo{
					{Name: "MA", Email: "ma@example.com", Username: "mauser", Avatar: "ma.png"},
				},
				ProgramManager:   &events.UserInfo{Name: "PM", Email: "pm@example.com", Username: "pmuser", Avatar: "pm.png"},
				OpportunityOwner: &events.UserInfo{Name: "OO", Email: "oo@example.com", Username: "oouser", Avatar: "oo.png"},
				CreatedAt:        &now,
				UpdatedAt:        &now,
			},
		},
		{
			name: "nil optional pointers produce nil in output",
			input: &models.ProjectSettings{
				UID:              "uid-2",
				MissionStatement: "no optionals",
			},
			expected: events.ProjectSettings{
				UID:              "uid-2",
				MissionStatement: "no optionals",
			},
		},
		{
			name: "nil user slice preserves nil (serializes as JSON null not [])",
			input: &models.ProjectSettings{
				UID:     "uid-3",
				Writers: nil,
			},
			expected: events.ProjectSettings{
				UID: "uid-3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DomainSettingsToEvent(tt.input)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			assert.Equal(t, tt.expected.AnnouncementDate, result.AnnouncementDate)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.MeetingCoordinators, result.MeetingCoordinators)
			assert.Equal(t, tt.expected.ExecutiveDirector, result.ExecutiveDirector)
			assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
			assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
			assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt)
			assert.Equal(t, tt.expected.UpdatedAt, result.UpdatedAt)
		})
	}
}

func TestDomainDocumentToEvent(t *testing.T) {
	folderUID := "folder-1"
	tests := []struct {
		name     string
		input    *models.ProjectDocument
		expected events.ProjectDocumentCreatedMessage
	}{
		{
			name: "all fields mapped — no folder",
			input: &models.ProjectDocument{
				UID:        "doc-1",
				ProjectUID: "proj-1",
				Name:       "Charter",
				FileName:   "charter.pdf",
				CreatedBy:  &models.UserInfo{Username: "alice"},
			},
			expected: events.ProjectDocumentCreatedMessage{
				DocumentUID: "doc-1",
				ProjectUID:  "proj-1",
				Name:        "Charter",
				FileName:    "charter.pdf",
				FolderUID:   "",
				CreatedBy:   "alice",
			},
		},
		{
			name: "nil FolderUID coerced to empty string",
			input: &models.ProjectDocument{
				UID:        "doc-2",
				ProjectUID: "proj-2",
				Name:       "Spec",
				FileName:   "spec.pdf",
				FolderUID:  nil,
				CreatedBy:  &models.UserInfo{Username: "bob"},
			},
			expected: events.ProjectDocumentCreatedMessage{
				DocumentUID: "doc-2",
				ProjectUID:  "proj-2",
				Name:        "Spec",
				FileName:    "spec.pdf",
				FolderUID:   "",
				CreatedBy:   "bob",
			},
		},
		{
			name: "non-nil FolderUID passed through",
			input: &models.ProjectDocument{
				UID:        "doc-3",
				ProjectUID: "proj-3",
				Name:       "Report",
				FileName:   "report.pdf",
				FolderUID:  &folderUID,
				CreatedBy:  &models.UserInfo{Username: "carol"},
			},
			expected: events.ProjectDocumentCreatedMessage{
				DocumentUID: "doc-3",
				ProjectUID:  "proj-3",
				Name:        "Report",
				FileName:    "report.pdf",
				FolderUID:   "folder-1",
				CreatedBy:   "carol",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DomainDocumentToEvent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDomainLinkToEvent(t *testing.T) {
	folderUID := "folder-1"
	tests := []struct {
		name     string
		input    *models.ProjectLink
		expected events.ProjectLinkCreatedMessage
	}{
		{
			name: "all fields mapped — no folder",
			input: &models.ProjectLink{
				UID:        "link-1",
				ProjectUID: "proj-1",
				Name:       "Governance",
				URL:        "https://example.com/governance",
				CreatedBy:  &models.UserInfo{Username: "alice"},
			},
			expected: events.ProjectLinkCreatedMessage{
				LinkUID:    "link-1",
				ProjectUID: "proj-1",
				Name:       "Governance",
				URL:        "https://example.com/governance",
				FolderUID:  "",
				CreatedBy:  "alice",
			},
		},
		{
			name: "nil FolderUID coerced to empty string",
			input: &models.ProjectLink{
				UID:        "link-2",
				ProjectUID: "proj-2",
				Name:       "Spec",
				URL:        "https://example.com/spec",
				FolderUID:  nil,
				CreatedBy:  &models.UserInfo{Username: "bob"},
			},
			expected: events.ProjectLinkCreatedMessage{
				LinkUID:    "link-2",
				ProjectUID: "proj-2",
				Name:       "Spec",
				URL:        "https://example.com/spec",
				FolderUID:  "",
				CreatedBy:  "bob",
			},
		},
		{
			name: "non-nil FolderUID passed through",
			input: &models.ProjectLink{
				UID:        "link-3",
				ProjectUID: "proj-3",
				Name:       "RFC",
				URL:        "https://example.com/rfc",
				FolderUID:  &folderUID,
				CreatedBy:  &models.UserInfo{Username: "carol"},
			},
			expected: events.ProjectLinkCreatedMessage{
				LinkUID:    "link-3",
				ProjectUID: "proj-3",
				Name:       "RFC",
				URL:        "https://example.com/rfc",
				FolderUID:  "folder-1",
				CreatedBy:  "carol",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DomainLinkToEvent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProjectProjection(t *testing.T) {
	base := &models.ProjectBase{
		UID:    "00000000-0000-0000-0000-000000000001",
		Slug:   "test-project",
		Name:   "Test Project",
		Public: true,
	}
	settings := &models.ProjectSettings{
		UID:     "00000000-0000-0000-0000-000000000001",
		Writers: []models.UserInfo{{Username: "alice"}},
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "ToFull delegates to ConvertToProjectFull",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, ConvertToProjectFull(base, settings), proj.ToFull())
			},
		},
		{
			name: "ToServiceBase delegates to ConvertToServiceProjectBase",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, ConvertToServiceProjectBase(base), proj.ToServiceBase())
			},
		},
		{
			name: "ToServiceSettings delegates to ConvertToServiceProjectSettings",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, ConvertToServiceProjectSettings(settings), proj.ToServiceSettings())
			},
		},
		{
			name: "ToEventSettings delegates to DomainSettingsToEvent",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, DomainSettingsToEvent(settings), proj.ToEventSettings())
			},
		},
		{
			name: "ToFGAMessage delegates to buildFGAUpdateAccessMessage",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, buildFGAUpdateAccessMessage(base, settings), proj.ToFGAMessage())
			},
		},
		{
			name: "ToFull with nil base returns nil",
			run: func(t *testing.T) {
				proj := NewProjectProjection(nil, settings)
				assert.Nil(t, proj.ToFull())
			},
		},
		{
			name: "ToEventSettings with nil settings returns empty struct",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, nil)
				assert.Equal(t, events.ProjectSettings{}, proj.ToEventSettings())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

func TestBuildFGAUpdateAccessMessage(t *testing.T) {
	base := &models.ProjectBase{UID: "project-1", Public: true}

	t.Run("includes resolvable writers and omits empty-username pending invitees", func(t *testing.T) {
		settings := &models.ProjectSettings{
			UID: "project-1",
			Writers: []models.UserInfo{
				{Username: "alice", Email: "alice@example.com"},
				{Email: "pending@example.com"},
			},
		}
		msg := buildFGAUpdateAccessMessage(base, settings)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, []string{"alice"}, data.Relations[fgaconstants.RelationWriter])
		_, hasAuditor := data.Relations[fgaconstants.RelationAuditor]
		assert.False(t, hasAuditor)
	})

	t.Run("omits writer key when the writers slice is empty", func(t *testing.T) {
		settings := &models.ProjectSettings{UID: "project-1"}
		msg := buildFGAUpdateAccessMessage(base, settings)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		_, hasWriter := data.Relations[fgaconstants.RelationWriter]
		assert.False(t, hasWriter)
	})

	t.Run("omits writer key when every writer is a pending invite", func(t *testing.T) {
		settings := &models.ProjectSettings{
			UID:     "project-1",
			Writers: []models.UserInfo{{Email: "pending@example.com"}},
		}
		msg := buildFGAUpdateAccessMessage(base, settings)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		_, hasWriter := data.Relations[fgaconstants.RelationWriter]
		assert.False(t, hasWriter)
	})

	t.Run("publishes mentorship project-admin relation during full sync", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(base, &models.ProjectSettings{
			UID:                     "project-1",
			MentorshipProgramAdmins: []models.UserInfo{{Username: "admin1"}, {Username: "admin2"}},
		})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, []string{"admin1", "admin2"}, data.Relations["mentorship_program_admin"])
		assert.Empty(t, data.ExcludeRelations)
	})
}
