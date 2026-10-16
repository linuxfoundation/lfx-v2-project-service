// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestUserInfo creates a UserInfo for testing purposes
func createTestUserInfo(username, name, email, avatar string) models.UserInfo {
	return models.UserInfo{
		Name:     name,
		Email:    email,
		Username: username,
		Avatar:   avatar,
	}
}

// createTestAPIUserInfo creates an API UserInfo for testing purposes
func createTestAPIUserInfo(username, name, email, avatar string) *projsvc.UserInfo {
	return &projsvc.UserInfo{
		Name:     &name,
		Email:    &email,
		Username: &username,
		Avatar:   &avatar,
	}
}

func TestParseOptionalDate(t *testing.T) {
	tests := []struct {
		name    string
		value   *string
		want    *time.Time
		wantErr bool
	}{
		{name: "nil is absent", value: nil, want: nil},
		{name: "empty is absent", value: misc.StringPtr(""), want: nil},
		{name: "whitespace is absent", value: misc.StringPtr("   "), want: nil},
		{
			name:  "valid date",
			value: misc.StringPtr("2023-12-31"),
			want:  func() *time.Time { d := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC); return &d }(),
		},
		{name: "invalid date is a validation error", value: misc.StringPtr("31/12/2023"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptionalDate("entity_dissolution_date", tt.value, time.DateOnly)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, domain.ErrValidationFailed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProjectBaseFromService(t *testing.T) {
	tests := []struct {
		name    string
		input   *projsvc.ProjectBase
		want    *models.ProjectBase
		wantErr bool
	}{
		{
			name: "valid project base conversion",
			input: &projsvc.ProjectBase{
				UID:          misc.StringPtr("test-uid"),
				Slug:         misc.StringPtr("test-slug"),
				Name:         misc.StringPtr("Test Project"),
				Description:  misc.StringPtr("Test Description"),
				Public:       misc.BoolPtr(true),
				ParentUID:    misc.StringPtr("parent-uid"),
				ContactEmail: misc.StringPtr("project-lead@example.com"),
				FundingModel: []string{"Crowdfunding"},
			},
			want: &models.ProjectBase{
				UID:          "test-uid",
				Slug:         "test-slug",
				Name:         "Test Project",
				Description:  "Test Description",
				Public:       true,
				ParentUID:    "parent-uid",
				ContactEmail: "project-lead@example.com",
				FundingModel: []string{"Crowdfunding"},
			},
		},
		{
			name:  "nil input",
			input: nil,
			want:  &models.ProjectBase{},
		},
		{
			name:  "absent optional fields become zero values",
			input: &projsvc.ProjectBase{UID: misc.StringPtr("test-uid")},
			want:  &models.ProjectBase{UID: "test-uid"},
		},
		{
			name: "with dates",
			input: &projsvc.ProjectBase{
				UID:                   misc.StringPtr("test-uid"),
				FormationDate:         misc.StringPtr("2020-01-15"),
				EntityDissolutionDate: misc.StringPtr("2023-12-31"),
			},
			want: &models.ProjectBase{
				UID:                   "test-uid",
				FormationDate:         func() *time.Time { d := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC); return &d }(),
				EntityDissolutionDate: func() *time.Time { d := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC); return &d }(),
			},
		},
		{
			name: "blank dissolution date is absent",
			input: &projsvc.ProjectBase{
				UID:                   misc.StringPtr("test-uid"),
				EntityDissolutionDate: misc.StringPtr("  "),
			},
			want: &models.ProjectBase{UID: "test-uid"},
		},
		{
			name: "invalid formation date",
			input: &projsvc.ProjectBase{
				UID:           misc.StringPtr("test-uid"),
				FormationDate: misc.StringPtr("invalid-date"),
			},
			wantErr: true,
		},
		{
			name: "invalid dissolution date",
			input: &projsvc.ProjectBase{
				UID:                   misc.StringPtr("test-uid"),
				EntityDissolutionDate: misc.StringPtr("invalid-date"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projectBaseFromService(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, domain.ErrValidationFailed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProjectSettingsInputFromService(t *testing.T) {
	announcement := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   *projsvc.ProjectSettings
		check   func(t *testing.T, got service.ProjectSettingsInput)
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			check: func(t *testing.T, got service.ProjectSettingsInput) {
				assert.Equal(t, service.ProjectSettingsInput{}, got)
			},
		},
		{
			name: "scalar fields and announcement date",
			input: &projsvc.ProjectSettings{
				MissionStatement: misc.StringPtr("Test Mission"),
				AnnouncementDate: misc.StringPtr("2023-06-01"),
			},
			check: func(t *testing.T, got service.ProjectSettingsInput) {
				assert.Equal(t, "Test Mission", got.MissionStatement)
				assert.Equal(t, &announcement, got.AnnouncementDate)
			},
		},
		{
			name:    "invalid announcement date",
			input:   &projsvc.ProjectSettings{AnnouncementDate: misc.StringPtr("invalid-date")},
			wantErr: true,
		},
		{
			name: "users keep their fields and drop the invite",
			input: &projsvc.ProjectSettings{
				Writers: []*projsvc.UserInfo{{
					Name:     misc.StringPtr("Writer One"),
					Email:    misc.StringPtr("writer1@example.com"),
					Username: misc.StringPtr("writer1"),
					Avatar:   misc.StringPtr("https://example.com/a.png"),
					Invite:   &projsvc.InviteInfo{UID: misc.StringPtr("inv-1")},
				}},
				ExecutiveDirector: createTestAPIUserInfo("ed1", "ED One", "ed1@example.com", ""),
			},
			check: func(t *testing.T, got service.ProjectSettingsInput) {
				require.Len(t, got.Writers, 1)
				assert.Equal(t, &service.UserInput{
					Name:     misc.StringPtr("Writer One"),
					Email:    misc.StringPtr("writer1@example.com"),
					Username: misc.StringPtr("writer1"),
					Avatar:   misc.StringPtr("https://example.com/a.png"),
				}, got.Writers[0])
				require.NotNil(t, got.ExecutiveDirector)
				assert.Equal(t, "ed1", *got.ExecutiveDirector.Username)
			},
		},
		{
			name: "an omitted role stays nil and a cleared role stays empty",
			input: &projsvc.ProjectSettings{
				Writers:  []*projsvc.UserInfo{},
				Auditors: nil,
			},
			check: func(t *testing.T, got service.ProjectSettingsInput) {
				assert.NotNil(t, got.Writers)
				assert.Empty(t, got.Writers)
				assert.Nil(t, got.Auditors)
				assert.Nil(t, got.MentorshipProgramAdmins)
				assert.Nil(t, got.ExecutiveDirector)
			},
		},
		{
			name:  "a nil entry stays nil",
			input: &projsvc.ProjectSettings{Writers: []*projsvc.UserInfo{nil}},
			check: func(t *testing.T, got service.ProjectSettingsInput) {
				require.Len(t, got.Writers, 1)
				assert.Nil(t, got.Writers[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projectSettingsInputFromService(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, domain.ErrValidationFailed)
				return
			}
			require.NoError(t, err)
			tt.check(t, got)
		})
	}
}

func TestCreateProjectInputs(t *testing.T) {
	t.Run("maps base and settings from the payload", func(t *testing.T) {
		base, settings, err := createProjectInputs(&projsvc.CreateProjectPayload{
			Slug:                  "test-slug",
			Name:                  "Test Project",
			Description:           "Test Description",
			ParentUID:             "parent-uid",
			Stage:                 misc.StringPtr("Archived"),
			EntityDissolutionDate: misc.StringPtr("2023-12-31"),
			MissionStatement:      misc.StringPtr("Mission"),
			Writers:               []*projsvc.UserInfo{createTestAPIUserInfo("w1", "W One", "w1@example.com", "")},
		})
		require.NoError(t, err)
		assert.Empty(t, base.UID, "the service assigns the UID")
		assert.Equal(t, "test-slug", base.Slug)
		assert.Equal(t, "Test Project", base.Name)
		assert.Equal(t, "parent-uid", base.ParentUID)
		assert.Equal(t, "Archived", base.Stage)
		require.NotNil(t, base.EntityDissolutionDate)
		assert.Equal(t, "Mission", settings.MissionStatement)
		require.Len(t, settings.Writers, 1)
	})

	t.Run("a malformed date is a validation error", func(t *testing.T) {
		_, _, err := createProjectInputs(&projsvc.CreateProjectPayload{FormationDate: misc.StringPtr("nope")})
		assert.ErrorIs(t, err, domain.ErrValidationFailed)
		_, _, err = createProjectInputs(&projsvc.CreateProjectPayload{AnnouncementDate: misc.StringPtr("nope")})
		assert.ErrorIs(t, err, domain.ErrValidationFailed)
	})
}

func TestUpdateProjectBaseInput(t *testing.T) {
	t.Run("maps the payload including the UID", func(t *testing.T) {
		base, err := updateProjectBaseInput(&projsvc.UpdateProjectBasePayload{
			UID:           misc.StringPtr("project-uid"),
			Slug:          "test-slug",
			Name:          "Test Project",
			FormationDate: misc.StringPtr("2020-01-15"),
		})
		require.NoError(t, err)
		assert.Equal(t, "project-uid", base.UID)
		assert.Equal(t, "test-slug", base.Slug)
		require.NotNil(t, base.FormationDate)
	})

	t.Run("a malformed date is a validation error", func(t *testing.T) {
		_, err := updateProjectBaseInput(&projsvc.UpdateProjectBasePayload{EntityDissolutionDate: misc.StringPtr("nope")})
		assert.ErrorIs(t, err, domain.ErrValidationFailed)
	})
}

func TestUpdateProjectSettingsInput(t *testing.T) {
	t.Run("keeps an omitted mentorship admins list nil", func(t *testing.T) {
		settings, err := updateProjectSettingsInput(&projsvc.UpdateProjectSettingsPayload{
			UID:      misc.StringPtr("project-uid"),
			Writers:  []*projsvc.UserInfo{createTestAPIUserInfo("w1", "W One", "w1@example.com", "")},
			Auditors: []*projsvc.UserInfo{},
		})
		require.NoError(t, err)
		assert.Nil(t, settings.MentorshipProgramAdmins)
		assert.NotNil(t, settings.Auditors)
		require.Len(t, settings.Writers, 1)
	})

	t.Run("a malformed announcement date is a validation error", func(t *testing.T) {
		_, err := updateProjectSettingsInput(&projsvc.UpdateProjectSettingsPayload{AnnouncementDate: misc.StringPtr("nope")})
		assert.ErrorIs(t, err, domain.ErrValidationFailed)
	})
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
				ContactEmail:  "project-lead@example.com",
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
				MentorshipProgramAdmins: []models.UserInfo{
					createTestUserInfo("mentor1", "Mentor One", "mentor1@example.com", ""),
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
				MentorshipProgramAdmins: []*projsvc.UserInfo{
					createTestAPIUserInfo("mentor1", "Mentor One", "mentor1@example.com", ""),
				},
				ContactEmail: misc.StringPtr("project-lead@example.com"),
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
			result := convertToProjectFull(tt.base, tt.settings)
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
				assert.Equal(t, tt.expected.MentorshipProgramAdmins, result.MentorshipProgramAdmins)
				assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
				assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
				assert.Equal(t, tt.expected.ContactEmail, result.ContactEmail)
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
				ContactEmail:    "project-lead@example.com",
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
				ContactEmail:    misc.StringPtr("project-lead@example.com"),
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
			result := convertToServiceProjectBase(tt.input)
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
			assert.Equal(t, tt.expected.ContactEmail, result.ContactEmail)
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
				MentorshipProgramAdmins: []models.UserInfo{
					createTestUserInfo("mentor1", "Mentor One", "mentor1@example.com", ""),
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
				MentorshipProgramAdmins: []*projsvc.UserInfo{
					createTestAPIUserInfo("mentor1", "Mentor One", "mentor1@example.com", ""),
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
			result := convertToServiceProjectSettings(tt.input)
			assert.Equal(t, tt.expected.UID, result.UID)
			if tt.expected.MissionStatement != nil {
				assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			}
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)
			assert.Equal(t, tt.expected.MeetingCoordinators, result.MeetingCoordinators)
			assert.Equal(t, tt.expected.MentorshipProgramAdmins, result.MentorshipProgramAdmins)
			assert.Equal(t, tt.expected.ProgramManager, result.ProgramManager)
			assert.Equal(t, tt.expected.OpportunityOwner, result.OpportunityOwner)
		})
	}
}

func TestConvertAuditUserToAPI(t *testing.T) {
	tests := []struct {
		name          string
		input         *models.UserInfo
		wantNil       bool
		wantName      string
		wantUsername  string
		wantAvatar    string
		wantEmailNone bool
	}{
		{
			name:    "nil input returns nil",
			input:   nil,
			wantNil: true,
		},
		{
			name: "email is omitted; name username avatar are mapped",
			input: &models.UserInfo{
				Name:     "Alice Example",
				Username: "alice",
				Avatar:   "https://cdn.example/avatar.png",
				Email:    "alice@example.com",
			},
			wantName:      "Alice Example",
			wantUsername:  "alice",
			wantAvatar:    "https://cdn.example/avatar.png",
			wantEmailNone: true,
		},
		{
			name: "empty email on input is also absent on output",
			input: &models.UserInfo{
				Name:     "Bob Fixture",
				Username: "bob",
			},
			wantName:      "Bob Fixture",
			wantUsername:  "bob",
			wantEmailNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertAuditUserToAPI(tt.input)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, *got.Name)
			assert.Equal(t, tt.wantUsername, *got.Username)
			assert.Equal(t, tt.wantAvatar, *got.Avatar)
			if tt.wantEmailNone {
				// AuditUserInfo has no Email field — verify by checking the struct type.
				type hasEmail interface{ GetEmail() string }
				_, hasEmailField := any(got).(hasEmail)
				assert.False(t, hasEmailField, "AuditUserInfo must not expose an email field")
			}
		})
	}
}

func TestConvertToServiceDocument(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	user := &models.UserInfo{Name: "Alice Example", Username: "alice", Email: "alice@example.com"}

	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, convertToServiceDocument(nil))
	})

	t.Run("all fields are mapped", func(t *testing.T) {
		got := convertToServiceDocument(&models.ProjectDocument{
			UID:         "doc-1",
			ProjectUID:  "proj-1",
			FolderUID:   misc.StringPtr("folder-1"),
			Name:        "Charter",
			Description: "The charter",
			FileName:    "charter.pdf",
			FileSize:    1024,
			ContentType: "application/pdf",
			CreatedBy:   user,
			UpdatedBy:   user,
			CreatedAt:   created,
			UpdatedAt:   updated,
		})
		require.NotNil(t, got)
		assert.Equal(t, "doc-1", *got.UID)
		assert.Equal(t, "proj-1", *got.ProjectUID)
		assert.Equal(t, "folder-1", *got.FolderUID)
		assert.Equal(t, "Charter", *got.Name)
		assert.Equal(t, "The charter", *got.Description)
		assert.Equal(t, "charter.pdf", *got.FileName)
		assert.Equal(t, int64(1024), *got.FileSize)
		assert.Equal(t, "application/pdf", *got.ContentType)
		assert.Equal(t, "alice", *got.CreatedBy.Username)
		assert.Equal(t, "alice", *got.UpdatedBy.Username)
		assert.Equal(t, "2026-01-02T03:04:05Z", *got.CreatedAt)
		assert.Equal(t, "2026-02-03T04:05:06Z", *got.UpdatedAt)
	})

	t.Run("empty optional fields are omitted", func(t *testing.T) {
		got := convertToServiceDocument(&models.ProjectDocument{UID: "doc-1", CreatedAt: created, UpdatedAt: updated})
		require.NotNil(t, got)
		assert.Nil(t, got.Description)
		assert.Nil(t, got.ContentType)
		assert.Nil(t, got.FolderUID)
		assert.Nil(t, got.CreatedBy)
	})
}

func TestConvertToServiceLink(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	user := &models.UserInfo{Name: "Alice Example", Username: "alice", Email: "alice@example.com"}

	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, convertToServiceLink(nil))
	})

	t.Run("all fields are mapped", func(t *testing.T) {
		got := convertToServiceLink(&models.ProjectLink{
			UID:         "link-1",
			ProjectUID:  "proj-1",
			FolderUID:   misc.StringPtr("folder-1"),
			Name:        "Website",
			URL:         "https://example.com",
			Description: "Project site",
			CreatedBy:   user,
			UpdatedBy:   user,
			CreatedAt:   created,
			UpdatedAt:   updated,
		})
		require.NotNil(t, got)
		assert.Equal(t, "link-1", *got.UID)
		assert.Equal(t, "proj-1", *got.ProjectUID)
		assert.Equal(t, "folder-1", *got.FolderUID)
		assert.Equal(t, "Website", *got.Name)
		assert.Equal(t, "https://example.com", *got.URL)
		assert.Equal(t, "Project site", *got.Description)
		assert.Equal(t, "alice", *got.CreatedBy.Username)
		assert.Equal(t, "2026-01-02T03:04:05Z", *got.CreatedAt)
		assert.Equal(t, "2026-02-03T04:05:06Z", *got.UpdatedAt)
	})

	t.Run("empty description is omitted", func(t *testing.T) {
		got := convertToServiceLink(&models.ProjectLink{UID: "link-1", CreatedAt: created, UpdatedAt: updated})
		require.NotNil(t, got)
		assert.Nil(t, got.Description)
		assert.Nil(t, got.FolderUID)
	})
}

func TestConvertToServiceFolder(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	user := &models.UserInfo{Name: "Alice Example", Username: "alice", Email: "alice@example.com"}

	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, convertToServiceFolder(nil))
	})

	t.Run("all fields are mapped", func(t *testing.T) {
		got := convertToServiceFolder(&models.ProjectFolder{
			UID:        "folder-1",
			ProjectUID: "proj-1",
			Name:       "Legal",
			CreatedBy:  user,
			UpdatedBy:  user,
			CreatedAt:  created,
			UpdatedAt:  updated,
		})
		require.NotNil(t, got)
		assert.Equal(t, "folder-1", *got.UID)
		assert.Equal(t, "proj-1", *got.ProjectUID)
		assert.Equal(t, "Legal", *got.Name)
		assert.Equal(t, "alice", *got.CreatedBy.Username)
		assert.Equal(t, "alice", *got.UpdatedBy.Username)
		assert.Equal(t, "2026-01-02T03:04:05Z", *got.CreatedAt)
		assert.Equal(t, "2026-02-03T04:05:06Z", *got.UpdatedAt)
	})
}
