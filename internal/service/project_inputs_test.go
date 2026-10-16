// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
)

// testUserInput creates a fully populated UserInput for testing purposes.
func testUserInput(username, name, email, avatar string) *UserInput {
	return &UserInput{
		Name:     &name,
		Email:    &email,
		Username: &username,
		Avatar:   &avatar,
	}
}

func TestBuildProjectSettings(t *testing.T) {
	announcement := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    ProjectSettingsInput
		existing *models.ProjectSettings // simulates the stored record on a PUT
		expected *models.ProjectSettings
	}{
		{
			name: "valid project settings conversion",
			input: ProjectSettingsInput{
				MissionStatement: "Test Mission",
				Writers: []*UserInput{
					testUserInput("writer1", "Writer One", "writer1@example.com", ""),
					testUserInput("writer2", "Writer Two", "writer2@example.com", ""),
				},
				Auditors: []*UserInput{
					testUserInput("auditor1", "Auditor One", "auditor1@example.com", ""),
				},
				MeetingCoordinators: []*UserInput{
					testUserInput("coordinator1", "Coordinator One", "coordinator1@example.com", ""),
				},
			},
			expected: &models.ProjectSettings{
				MissionStatement: "Test Mission",
				Writers: []models.UserInfo{
					{Username: "writer1", Name: "Writer One", Email: "writer1@example.com"},
					{Username: "writer2", Name: "Writer Two", Email: "writer2@example.com"},
				},
				Auditors: []models.UserInfo{
					{Username: "auditor1", Name: "Auditor One", Email: "auditor1@example.com"},
				},
				MeetingCoordinators: []models.UserInfo{
					{Username: "coordinator1", Name: "Coordinator One", Email: "coordinator1@example.com"},
				},
			},
		},
		{
			name:     "empty input",
			input:    ProjectSettingsInput{},
			expected: &models.ProjectSettings{},
		},
		{
			name:     "announcement date",
			input:    ProjectSettingsInput{AnnouncementDate: &announcement},
			expected: &models.ProjectSettings{AnnouncementDate: &announcement},
		},
		{
			name: "with program_manager and opportunity_owner",
			input: ProjectSettingsInput{
				ProgramManager:   testUserInput("pm1", "PM One", "pm1@example.com", ""),
				OpportunityOwner: testUserInput("oo1", "OO One", "oo1@example.com", ""),
			},
			expected: &models.ProjectSettings{
				ProgramManager:   &models.UserInfo{Username: "pm1", Name: "PM One", Email: "pm1@example.com"},
				OpportunityOwner: &models.UserInfo{Username: "oo1", Name: "OO One", Email: "oo1@example.com"},
			},
		},
		{
			name: "with mentorship program admins",
			input: ProjectSettingsInput{
				MentorshipProgramAdmins: []*UserInput{
					testUserInput("mentor1", "Mentor One", "mentor1@example.com", ""),
				},
			},
			expected: &models.ProjectSettings{
				MentorshipProgramAdmins: []models.UserInfo{
					{Username: "mentor1", Name: "Mentor One", Email: "mentor1@example.com"},
				},
			},
		},
		{
			name: "mentorship admin stored LFID preserved on email lookup miss (empty username)",
			existing: &models.ProjectSettings{
				MentorshipProgramAdmins: []models.UserInfo{{Username: "stored-mentor", Email: "mentor@example.com"}},
			},
			input: ProjectSettingsInput{
				MentorshipProgramAdmins: []*UserInput{
					{Name: misc.StringPtr("Mentor"), Email: misc.StringPtr("mentor@example.com"), Username: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				MentorshipProgramAdmins: []models.UserInfo{{Name: "Mentor", Email: "mentor@example.com", Username: "stored-mentor"}},
			},
		},
		{
			name: "invite preserved when user still in list (PUT does not wipe invite)",
			existing: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Email: "w@example.com", Name: "Old Name", Invite: &models.InviteInfo{UID: "inv-1", Email: "w@example.com"}},
				},
			},
			input: ProjectSettingsInput{
				Writers: []*UserInput{
					{Name: misc.StringPtr("New Name"), Email: misc.StringPtr("w@example.com"), Username: misc.StringPtr(""), Avatar: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Name: "New Name", Email: "w@example.com", Invite: &models.InviteInfo{UID: "inv-1", Email: "w@example.com"}},
				},
			},
		},
		{
			name: "empty username preserves stored LFID",
			existing: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Email: "gone@example.com", Username: "stored-lfid", Name: "Old Name"},
				},
			},
			input: ProjectSettingsInput{
				Writers: []*UserInput{
					{Name: misc.StringPtr("Old Name"), Email: misc.StringPtr("gone@example.com"), Username: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Name: "Old Name", Email: "gone@example.com", Username: "stored-lfid"},
				},
			},
		},
		{
			name: "an omitted field keeps the stored value and an explicit empty one replaces it",
			existing: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Email: "w@example.com", Name: "Stored Name", Avatar: "https://example.com/stored.png"},
				},
			},
			input: ProjectSettingsInput{
				Writers: []*UserInput{
					{Email: misc.StringPtr("w@example.com"), Avatar: misc.StringPtr("")},
				},
			},
			expected: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Email: "w@example.com", Name: "Stored Name", Avatar: ""},
				},
			},
		},
		{
			name: "empty executive director username preserves stored LFID",
			existing: &models.ProjectSettings{
				ExecutiveDirector: &models.UserInfo{
					Email:    "ed@example.com",
					Username: "stored-ed",
					Name:     "Old ED",
				},
			},
			input: ProjectSettingsInput{
				ExecutiveDirector: &UserInput{
					Name:     misc.StringPtr("Old ED"),
					Email:    misc.StringPtr("ed@example.com"),
					Username: misc.StringPtr(""),
				},
			},
			expected: &models.ProjectSettings{
				ExecutiveDirector: &models.UserInfo{
					Name:     "Old ED",
					Email:    "ed@example.com",
					Username: "stored-ed",
				},
			},
		},
		{
			name: "stored record of a different user is not used to seed a single role",
			existing: &models.ProjectSettings{
				ProgramManager: &models.UserInfo{Email: "old-pm@example.com", Username: "old-pm", Invite: &models.InviteInfo{UID: "inv-3"}},
			},
			input: ProjectSettingsInput{
				ProgramManager: &UserInput{Name: misc.StringPtr("New PM"), Email: misc.StringPtr("new-pm@example.com")},
			},
			expected: &models.ProjectSettings{
				ProgramManager: &models.UserInfo{Name: "New PM", Email: "new-pm@example.com"},
			},
		},
		{
			name: "invite gone when user removed from list",
			existing: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{Email: "gone@example.com", Invite: &models.InviteInfo{UID: "inv-2", Email: "gone@example.com"}},
				},
			},
			input: ProjectSettingsInput{
				Writers: []*UserInput{}, // user removed
			},
			expected: &models.ProjectSettings{
				Writers: nil,
			},
		},
		{
			name: "a nil entry becomes an empty user and keeps the list length",
			input: ProjectSettingsInput{
				Writers: []*UserInput{nil, testUserInput("writer1", "Writer One", "writer1@example.com", "")},
			},
			expected: &models.ProjectSettings{
				Writers: []models.UserInfo{
					{},
					{Username: "writer1", Name: "Writer One", Email: "writer1@example.com"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProjectSettings(tt.input, tt.existing)
			assert.Equal(t, tt.expected, result)
		})
	}
}
