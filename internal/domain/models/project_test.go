// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProjectBaseTags(t *testing.T) {
	tests := []struct {
		name     string
		project  *ProjectBase
		expected []string
	}{
		{
			name:     "nil project",
			project:  nil,
			expected: nil,
		},
		{
			name:     "empty project",
			project:  &ProjectBase{},
			expected: nil,
		},
		{
			name: "project with UID only",
			project: &ProjectBase{
				UID: "project-123",
			},
			expected: []string{
				"project-123",
				"project_uid:project-123",
			},
		},
		{
			name: "project with Slug only",
			project: &ProjectBase{
				Slug: "test-project",
			},
			expected: []string{
				"test-project",
				"project_slug:test-project",
			},
		},
		{
			name: "project with Name only",
			project: &ProjectBase{
				Name: "Test Project",
			},
			expected: []string{
				"Test Project",
			},
		},
		{
			name: "project with Description only",
			project: &ProjectBase{
				Description: "This is a test project",
			},
			expected: []string{
				"This is a test project",
			},
		},
		{
			name: "project with all tag fields",
			project: &ProjectBase{
				UID:         "project-123",
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "This is a test project",
				ParentUID:   "parent-456",
			},
			expected: []string{
				"project-123",
				"project_uid:project-123",
				"project_parent_uid:parent-456",
				"test-project",
				"project_slug:test-project",
				"Test Project",
				"This is a test project",
			},
		},
		{
			name: "project with ParentUID",
			project: &ProjectBase{
				UID:       "project-123",
				ParentUID: "parent-456",
			},
			expected: []string{
				"project-123",
				"project_uid:project-123",
				"project_parent_uid:parent-456",
			},
		},
		{
			name: "project with all fields including non-tag fields",
			project: &ProjectBase{
				UID:                        "project-123",
				Slug:                       "test-project",
				Name:                       "Test Project",
				Description:                "This is a test project",
				Public:                     true,
				ParentUID:                  "parent-456",
				Stage:                      "incubating",
				Category:                   "software",
				LegalEntityType:            "foundation",
				LegalEntityName:            "Test Foundation",
				LegalParentUID:             "legal-789",
				FundingModel:               []string{"membership", "donation"},
				EntityFormationDocumentURL: "https://example.com/docs",
				AutojoinEnabled:            true,
				CharterURL:                 "https://example.com/charter",
				LogoURL:                    "https://example.com/logo.png",
				WebsiteURL:                 "https://example.com",
				RepositoryURL:              "https://github.com/example/repo",
			},
			expected: []string{
				"project-123",
				"project_uid:project-123",
				"project_parent_uid:parent-456",
				"test-project",
				"project_slug:test-project",
				"Test Project",
				"This is a test project",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := tt.project.Tags()
			// Check length first
			assert.Equal(t, len(tt.expected), len(tags), "Tag count mismatch")

			// Check each expected tag is present in the result
			for _, expectedTag := range tt.expected {
				assert.Contains(t, tags, expectedTag, "Expected tag %s not found", expectedTag)
			}

			// Check each result tag is present in the expected list
			for _, resultTag := range tags {
				assert.Contains(t, tt.expected, resultTag, "Unexpected tag %s found", resultTag)
			}
		})
	}
}

func TestProjectSettingsTags(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		settings *ProjectSettings
		expected []string
	}{
		{
			name:     "nil settings",
			settings: nil,
			expected: nil,
		},
		{
			name:     "empty settings",
			settings: &ProjectSettings{},
			expected: nil,
		},
		{
			name: "settings with UID only",
			settings: &ProjectSettings{
				UID: "settings-123",
			},
			expected: []string{
				"settings-123",
				"project_uid:settings-123",
			},
		},
		{
			name: "settings with MissionStatement only",
			settings: &ProjectSettings{
				MissionStatement: "Our mission is to test",
			},
			expected: []string{
				"Our mission is to test",
			},
		},
		{
			name: "settings with all tag fields",
			settings: &ProjectSettings{
				UID:              "settings-123",
				MissionStatement: "Our mission is to test",
			},
			expected: []string{
				"settings-123",
				"project_uid:settings-123",
				"Our mission is to test",
			},
		},
		{
			name: "settings with all fields including non-tag fields",
			settings: &ProjectSettings{
				UID:                 "settings-123",
				MissionStatement:    "Our mission is to test",
				AnnouncementDate:    &now,
				Auditors:            []string{"auditor1", "auditor2"},
				Writers:             []string{"writer1", "writer2"},
				MeetingCoordinators: []string{"coordinator1"},
				CreatedAt:           &now,
				UpdatedAt:           &now,
			},
			expected: []string{
				"settings-123",
				"project_uid:settings-123",
				"Our mission is to test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := tt.settings.Tags()
			assert.Equal(t, tt.expected, tags)
		})
	}
}
