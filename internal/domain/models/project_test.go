// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			expected: nil,
		},
		{
			name: "project with Slug only",
			project: &ProjectBase{
				Slug: "test-project",
			},
			expected: []string{
				"project_slug:test-project",
			},
		},
		{
			name: "project with Name only",
			project: &ProjectBase{
				Name: "Test Project",
			},
			expected: nil,
		},
		{
			name: "project with Description only",
			project: &ProjectBase{
				Description: "This is a test project",
			},
			expected: nil,
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
				"project_slug:test-project",
			},
		},
		{
			name: "project with ParentUID",
			project: &ProjectBase{
				UID:       "project-123",
				ParentUID: "parent-456",
			},
			expected: nil,
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
				"project_slug:test-project",
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
			expected: []string{},
		},
		{
			name: "settings with UID only",
			settings: &ProjectSettings{
				UID: "settings-123",
			},
			expected: []string{},
		},
		{
			name: "settings with MissionStatement only",
			settings: &ProjectSettings{
				MissionStatement: "Our mission is to test",
			},
			expected: []string{},
		},
		{
			name: "settings with all tag fields",
			settings: &ProjectSettings{
				UID:              "settings-123",
				MissionStatement: "Our mission is to test",
			},
			expected: []string{},
		},
		{
			name: "settings with all fields including non-tag fields",
			settings: &ProjectSettings{
				UID:              "settings-123",
				MissionStatement: "Our mission is to test",
				AnnouncementDate: &now,
				Auditors: []UserInfo{
					{Name: "Auditor One", Email: "auditor1@example.com", Username: "auditor1", Avatar: "https://example.com/avatar1.jpg"},
					{Name: "Auditor Two", Email: "auditor2@example.com", Username: "auditor2", Avatar: "https://example.com/avatar2.jpg"},
				},
				Writers: []UserInfo{
					{Name: "Writer One", Email: "writer1@example.com", Username: "writer1", Avatar: "https://example.com/avatar3.jpg"},
					{Name: "Writer Two", Email: "writer2@example.com", Username: "writer2", Avatar: "https://example.com/avatar4.jpg"},
				},
				MeetingCoordinators: []UserInfo{
					{Name: "Coordinator One", Email: "coordinator1@example.com", Username: "coordinator1", Avatar: "https://example.com/avatar5.jpg"},
				},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := tt.settings.Tags()
			assert.Equal(t, tt.expected, tags)
		})
	}
}

func TestProjectBase_IndexingConfig(t *testing.T) {
	t.Run("nil project returns nil", func(t *testing.T) {
		var p *ProjectBase
		assert.Nil(t, p.IndexingConfig())
	})

	t.Run("populates templatized fields", func(t *testing.T) {
		p := &ProjectBase{Public: true, Name: "Test Project", Slug: "test-project", Description: "desc", ParentUID: "parent-1"}
		cfg := p.IndexingConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, "{{ uid }}", cfg.ObjectID)
		require.NotNil(t, cfg.Public)
		assert.True(t, *cfg.Public)
		assert.Equal(t, "project:{{ uid }}", cfg.AccessCheckObject)
		assert.Equal(t, "viewer", cfg.AccessCheckRelation)
		assert.Equal(t, "project:{{ uid }}", cfg.HistoryCheckObject)
		assert.Equal(t, "writer_guard", cfg.HistoryCheckRelation)
		assert.Equal(t, "{{ name }}", cfg.SortName)
		assert.Equal(t, []string{"{{ name }}", "{{ slug }}"}, cfg.NameAndAliases)
		assert.Equal(t, []string{"project:{{ parent_uid }}"}, cfg.ParentRefs)
		assert.Equal(t, "{{ name }} {{ slug }} {{ description }}", cfg.Fulltext)
		assert.Equal(t, []string{"project_slug:{{ slug }}"}, cfg.Tags)
	})
}

func TestProjectBase_TagsTemplatized(t *testing.T) {
	tests := []struct {
		name string
		obj  *ProjectBase
		want []string
	}{
		{name: "nil project", obj: nil, want: nil},
		{name: "empty project", obj: &ProjectBase{}, want: nil},
		{name: "slug set", obj: &ProjectBase{Slug: "test-project"}, want: []string{"project_slug:{{ slug }}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.obj.TagsTemplatized())
		})
	}
}

func TestProjectBase_ParentRefsTemplatized(t *testing.T) {
	tests := []struct {
		name string
		obj  *ProjectBase
		want []string
	}{
		{name: "nil project", obj: nil, want: nil},
		{name: "no parent", obj: &ProjectBase{}, want: nil},
		{name: "parent set", obj: &ProjectBase{ParentUID: "parent-1"}, want: []string{"project:{{ parent_uid }}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.obj.ParentRefsTemplatized())
		})
	}
}

func TestProjectBase_NameAndAliasesTemplatized(t *testing.T) {
	tests := []struct {
		name    string
		project *ProjectBase
		want    []string
	}{
		{name: "nil project", project: nil, want: nil},
		{name: "empty project", project: &ProjectBase{}, want: nil},
		{name: "name only", project: &ProjectBase{Name: "Test Project"}, want: []string{"{{ name }}"}},
		{name: "slug only", project: &ProjectBase{Slug: "test-project"}, want: []string{"{{ slug }}"}},
		{name: "name and slug", project: &ProjectBase{Name: "Test Project", Slug: "test-project"}, want: []string{"{{ name }}", "{{ slug }}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.project.NameAndAliasesTemplatized())
		})
	}
}

func TestProjectBase_FulltextTemplatized(t *testing.T) {
	tests := []struct {
		name    string
		project *ProjectBase
		want    string
	}{
		{name: "nil project", project: nil, want: ""},
		{name: "empty project", project: &ProjectBase{}, want: ""},
		{name: "name only", project: &ProjectBase{Name: "Test Project"}, want: "{{ name }}"},
		{name: "slug only", project: &ProjectBase{Slug: "test-project"}, want: "{{ slug }}"},
		{name: "description only", project: &ProjectBase{Description: "desc"}, want: "{{ description }}"},
		{name: "all fields", project: &ProjectBase{Name: "Test Project", Slug: "test-project", Description: "desc"}, want: "{{ name }} {{ slug }} {{ description }}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.project.FulltextTemplatized())
		})
	}
}

func TestProjectSettings_TagsTemplatized(t *testing.T) {
	tests := []struct {
		name     string
		settings *ProjectSettings
		want     []string
	}{
		{name: "nil settings", settings: nil, want: nil},
		{name: "empty settings", settings: &ProjectSettings{}, want: nil},
		{name: "uid only", settings: &ProjectSettings{UID: "p1"}, want: []string{"{{ uid }}"}},
		{name: "mission statement only", settings: &ProjectSettings{MissionStatement: "m"}, want: []string{"{{ mission_statement }}"}},
		{name: "uid and mission statement", settings: &ProjectSettings{UID: "p1", MissionStatement: "m"}, want: []string{"{{ uid }}", "{{ mission_statement }}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.settings.TagsTemplatized())
		})
	}
}

func TestProjectSettings_ParentRefsTemplatized(t *testing.T) {
	tests := []struct {
		name string
		obj  *ProjectSettings
		want []string
	}{
		{name: "nil settings", obj: nil, want: nil},
		{name: "no uid", obj: &ProjectSettings{}, want: nil},
		{name: "uid set", obj: &ProjectSettings{UID: "p1"}, want: []string{"project:{{ uid }}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.obj.ParentRefsTemplatized())
		})
	}
}

func TestProjectSettings_IndexingConfig(t *testing.T) {
	t.Run("nil settings returns nil", func(t *testing.T) {
		var p *ProjectSettings
		assert.Nil(t, p.IndexingConfig("p1"))
	})

	t.Run("access checks use the project uid, not the settings uid", func(t *testing.T) {
		p := &ProjectSettings{UID: "settings-1", MissionStatement: "m"}
		cfg := p.IndexingConfig("p1")
		require.NotNil(t, cfg)
		assert.Equal(t, "{{ uid }}", cfg.ObjectID)
		assert.Equal(t, "project:p1", cfg.AccessCheckObject)
		assert.Equal(t, "auditor_guard", cfg.AccessCheckRelation)
		assert.Equal(t, "project:p1", cfg.HistoryCheckObject)
		assert.Equal(t, "writer_guard", cfg.HistoryCheckRelation)
		assert.Equal(t, []string{"project:{{ uid }}"}, cfg.ParentRefs)
		assert.Equal(t, []string{"{{ uid }}", "{{ mission_statement }}"}, cfg.Tags)
	})
}
