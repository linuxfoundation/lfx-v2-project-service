// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"
	"time"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			name: "ToEventSettings delegates to DomainSettingsToEvent",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				assert.Equal(t, DomainSettingsToEvent(settings), proj.ToEventSettings())
			},
		},
		{
			name: "ToFGAMessageKeepingDirectUsers excludes only the settings-derived user relations",
			run: func(t *testing.T) {
				proj := NewProjectProjection(base, settings)
				plain, ok := proj.ToFGAMessage().Data.(fgatypes.GenericAccessData)
				require.True(t, ok)
				assert.Empty(t, plain.ExcludeRelations)
				keep, ok := proj.ToFGAMessageKeepingDirectUsers().Data.(fgatypes.GenericAccessData)
				require.True(t, ok)
				assert.ElementsMatch(t, []string{"writer", "auditor", "meeting_coordinator", "executive_director", "mentorship_program_admin"}, keep.ExcludeRelations)
				keep.ExcludeRelations = nil
				assert.Equal(t, plain, keep, "only the exclusions differ")
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
	})

	t.Run("omits mentorship admin key when the roster is empty", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(base, &models.ProjectSettings{UID: "project-1"})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		_, present := data.Relations["mentorship_program_admin"]
		assert.False(t, present)
	})

	t.Run("omits mentorship admin key when every admin is a pending invite", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(base, &models.ProjectSettings{
			UID:                     "project-1",
			MentorshipProgramAdmins: []models.UserInfo{{Email: "pending@example.com"}},
		})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		_, present := data.Relations["mentorship_program_admin"]
		assert.False(t, present)
	})

	t.Run("publishes only resolved mentorship admins", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(base, &models.ProjectSettings{
			UID: "project-1",
			MentorshipProgramAdmins: []models.UserInfo{
				{Username: "admin1"},
				{Email: "pending@example.com"},
			},
		})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, []string{"admin1"}, data.Relations["mentorship_program_admin"])
	})

	// Foundations are created under the root project, so once root-inherited grants are removed
	// the root must carry the ordinary global grants itself.
	t.Run("root project carries every global grant", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(&models.ProjectBase{UID: "root"}, &models.ProjectSettings{UID: "root"})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, wantAllGrants, data.References)
	})

	t.Run("carries global grants as team references next to the parent, never as user relations", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(
			&models.ProjectBase{UID: "project-1", ParentUID: "parent-1", Stage: "Active"},
			&models.ProjectSettings{UID: "project-1", Writers: []models.UserInfo{{Username: "alice"}}},
		)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, wantAllGrantsWithParent("parent-1"), data.References)
		assert.Equal(t, map[string][]string{"writer": {"alice"}}, data.Relations)
	})

	t.Run("ordinary stage grants auditor, writer and marketing plus both owners", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(&models.ProjectBase{UID: "project-1", Stage: "Active"}, &models.ProjectSettings{UID: "project-1"})
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, wantAllGrants, data.References)
	})

	t.Run("confidential stage grants the owners only", func(t *testing.T) {
		msg := buildFGAUpdateAccessMessage(
			&models.ProjectBase{UID: "project-1", Stage: "Formation - Confidential"},
			&models.ProjectSettings{UID: "project-1"},
		)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		assert.Equal(t, wantOwnerOnlyGrants, data.References)
	})

	// fga-sync deletes team tuples on global_* relations the message no longer carries, unless the
	// relation is excluded. So withdrawal needs the key absent and the relation not excluded.
	t.Run("moving into confidential withdraws the gated grants and moving out restores them", func(t *testing.T) {
		settings := &models.ProjectSettings{UID: "project-1"}
		build := func(stage string) fgatypes.GenericAccessData {
			msg := buildFGAUpdateAccessMessage(&models.ProjectBase{UID: "project-1", Stage: stage}, settings)
			data, ok := msg.Data.(fgatypes.GenericAccessData)
			require.True(t, ok)
			return data
		}

		before := build("Formation - Engaged")
		inside := build("Formation - Confidential")
		after := build("Formation - Engaged")

		assert.Equal(t, wantAllGrants, before.References)
		for _, relation := range []string{"global_writer", "global_auditor", "global_marketing_ops"} {
			_, present := inside.References[relation]
			assert.False(t, present, "%s still sent while confidential", relation)
			assert.NotContains(t, inside.ExcludeRelations, relation)
		}
		assert.Equal(t, wantOwnerOnlyGrants, inside.References)
		assert.Equal(t, wantAllGrants, after.References)
	})
}
