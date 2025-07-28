// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToDBProjectBase(t *testing.T) {
	tests := []struct {
		name     string
		input    *projsvc.ProjectBase
		expected *ProjectBase
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
				Stage:       misc.StringPtr("incubating"),
				Category:    misc.StringPtr("foundation"),
			},
			expected: &ProjectBase{
				UID:         "test-uid",
				Slug:        "test-slug",
				Name:        "Test Project",
				Description: "Test Description",
				Public:      true,
				ParentUID:   "parent-uid",
				Stage:       "incubating",
				Category:    "foundation",
			},
			wantErr: false,
		},
		{
			name: "project base with dates",
			input: &projsvc.ProjectBase{
				UID:             misc.StringPtr("test-uid"),
				Slug:            misc.StringPtr("test-slug"),
				Name:            misc.StringPtr("Test Project"),
				Description:     misc.StringPtr("Test Description"),
				FormationDate:   misc.StringPtr("2023-01-15"),
				AutojoinEnabled: misc.BoolPtr(false),
				LegalEntityType: misc.StringPtr("LLC"),
				LegalEntityName: misc.StringPtr("Test LLC"),
				FundingModel:    []string{"donations", "grants"},
				CharterURL:      misc.StringPtr("https://example.com/charter"),
				LogoURL:         misc.StringPtr("https://example.com/logo.png"),
				WebsiteURL:      misc.StringPtr("https://example.com"),
				RepositoryURL:   misc.StringPtr("https://github.com/test/repo"),
			},
			expected: &ProjectBase{
				UID:             "test-uid",
				Slug:            "test-slug",
				Name:            "Test Project",
				Description:     "Test Description",
				AutojoinEnabled: false,
				LegalEntityType: "LLC",
				LegalEntityName: "Test LLC",
				FundingModel:    []string{"donations", "grants"},
				CharterURL:      "https://example.com/charter",
				LogoURL:         "https://example.com/logo.png",
				WebsiteURL:      "https://example.com",
				RepositoryURL:   "https://github.com/test/repo",
			},
			wantErr: false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: &ProjectBase{},
			wantErr:  false,
		},
		{
			name: "empty required fields",
			input: &projsvc.ProjectBase{
				UID:  misc.StringPtr(""),
				Slug: misc.StringPtr(""),
				Name: misc.StringPtr(""),
			},
			expected: &ProjectBase{
				UID:  "",
				Slug: "",
				Name: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToDBProjectBase(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.Slug, result.Slug)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Public, result.Public)
			assert.Equal(t, tt.expected.ParentUID, result.ParentUID)
			assert.Equal(t, tt.expected.Stage, result.Stage)
			assert.Equal(t, tt.expected.Category, result.Category)
			assert.Equal(t, tt.expected.LegalEntityType, result.LegalEntityType)
			assert.Equal(t, tt.expected.LegalEntityName, result.LegalEntityName)
			assert.Equal(t, tt.expected.FundingModel, result.FundingModel)
			assert.Equal(t, tt.expected.AutojoinEnabled, result.AutojoinEnabled)
			assert.Equal(t, tt.expected.CharterURL, result.CharterURL)
			assert.Equal(t, tt.expected.LogoURL, result.LogoURL)
			assert.Equal(t, tt.expected.WebsiteURL, result.WebsiteURL)
			assert.Equal(t, tt.expected.RepositoryURL, result.RepositoryURL)

			// Verify timestamps are set for non-nil cases
			if tt.expected.UID != "" || tt.expected.Slug != "" || tt.expected.Name != "" {
				assert.NotNil(t, result.CreatedAt)
				assert.NotNil(t, result.UpdatedAt)
			}
		})
	}
}

func TestConvertToDBProjectSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    *projsvc.ProjectSettings
		expected *ProjectSettings
		wantErr  bool
	}{
		{
			name: "valid project settings conversion",
			input: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				MissionStatement: misc.StringPtr("Our mission is to test"),
				AnnouncementDate: misc.StringPtr("2023-06-01"),
				Writers:          []string{"writer1", "writer2"},
				Auditors:         []string{"auditor1", "auditor2"},
			},
			expected: &ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Our mission is to test",
				Writers:          []string{"writer1", "writer2"},
				Auditors:         []string{"auditor1", "auditor2"},
			},
			wantErr: false,
		},
		{
			name: "minimal project settings",
			input: &projsvc.ProjectSettings{
				UID: misc.StringPtr("test-uid"),
			},
			expected: &ProjectSettings{
				UID: "test-uid",
			},
			wantErr: false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: &ProjectSettings{},
			wantErr:  false,
		},
		{
			name: "empty UID",
			input: &projsvc.ProjectSettings{
				UID: misc.StringPtr(""),
			},
			expected: &ProjectSettings{
				UID: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToDBProjectSettings(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)

			// Verify timestamps are set for non-nil cases
			if tt.expected.UID != "" {
				assert.NotNil(t, result.CreatedAt)
				assert.NotNil(t, result.UpdatedAt)
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
		base     *ProjectBase
		settings *ProjectSettings
		expected *projsvc.ProjectFull
	}{
		{
			name: "complete project with base and settings",
			base: &ProjectBase{
				UID:             "test-uid",
				Slug:            "test-slug",
				Name:            "Test Project",
				Description:     "Test Description",
				Public:          true,
				ParentUID:       "parent-uid",
				Stage:           "incubating",
				Category:        "foundation",
				LegalEntityType: "LLC",
				LegalEntityName: "Test LLC",
				FundingModel:    []string{"donations"},
				FormationDate:   &formationDate,
				AutojoinEnabled: true,
				CharterURL:      "https://example.com/charter",
				LogoURL:         "https://example.com/logo.png",
				WebsiteURL:      "https://example.com",
				RepositoryURL:   "https://github.com/test/repo",
				CreatedAt:       &now,
				UpdatedAt:       &now,
			},
			settings: &ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Our mission",
				AnnouncementDate: &announcementDate,
				Writers:          []string{"writer1"},
				Auditors:         []string{"auditor1"},
			},
			expected: &projsvc.ProjectFull{
				UID:              misc.StringPtr("test-uid"),
				Slug:             misc.StringPtr("test-slug"),
				Name:             misc.StringPtr("Test Project"),
				Description:      misc.StringPtr("Test Description"),
				Public:           misc.BoolPtr(true),
				ParentUID:        misc.StringPtr("parent-uid"),
				Stage:            misc.StringPtr("incubating"),
				Category:         misc.StringPtr("foundation"),
				LegalEntityType:  misc.StringPtr("LLC"),
				LegalEntityName:  misc.StringPtr("Test LLC"),
				FundingModel:     []string{"donations"},
				FormationDate:    misc.StringPtr("2020-01-15"),
				AutojoinEnabled:  misc.BoolPtr(true),
				CharterURL:       misc.StringPtr("https://example.com/charter"),
				LogoURL:          misc.StringPtr("https://example.com/logo.png"),
				WebsiteURL:       misc.StringPtr("https://example.com"),
				RepositoryURL:    misc.StringPtr("https://github.com/test/repo"),
				MissionStatement: misc.StringPtr("Our mission"),
				AnnouncementDate: misc.StringPtr("2023-06-01"),
				Writers:          []string{"writer1"},
				Auditors:         []string{"auditor1"},
			},
		},
		{
			name: "project with base only",
			base: &ProjectBase{
				UID:         "test-uid",
				Slug:        "test-slug",
				Name:        "Test Project",
				Description: "Test Description",
				Public:      false,
				CreatedAt:   &now,
				UpdatedAt:   &now,
			},
			settings: nil,
			expected: &projsvc.ProjectFull{
				UID:                        misc.StringPtr("test-uid"),
				Slug:                       misc.StringPtr("test-slug"),
				Name:                       misc.StringPtr("Test Project"),
				Description:                misc.StringPtr("Test Description"),
				Public:                     misc.BoolPtr(false),
				ParentUID:                  misc.StringPtr(""),
				Stage:                      misc.StringPtr(""),
				Category:                   misc.StringPtr(""),
				LegalEntityType:            misc.StringPtr(""),
				LegalEntityName:            misc.StringPtr(""),
				LegalParentUID:             misc.StringPtr(""),
				EntityFormationDocumentURL: misc.StringPtr(""),
				AutojoinEnabled:            misc.BoolPtr(false),
				CharterURL:                 misc.StringPtr(""),
				LogoURL:                    misc.StringPtr(""),
				WebsiteURL:                 misc.StringPtr(""),
				RepositoryURL:              misc.StringPtr(""),
				CreatedAt:                  misc.StringPtr(now.Format(time.RFC3339)),
				UpdatedAt:                  misc.StringPtr(now.Format(time.RFC3339)),
			},
		},
		{
			name:     "nil base returns nil",
			base:     nil,
			settings: &ProjectSettings{UID: "test-uid"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToProjectFull(tt.base, tt.settings)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.Slug, result.Slug)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Public, result.Public)
			assert.Equal(t, tt.expected.ParentUID, result.ParentUID)
			assert.Equal(t, tt.expected.Stage, result.Stage)
			assert.Equal(t, tt.expected.Category, result.Category)
			assert.Equal(t, tt.expected.LegalEntityType, result.LegalEntityType)
			assert.Equal(t, tt.expected.LegalEntityName, result.LegalEntityName)
			assert.Equal(t, tt.expected.FundingModel, result.FundingModel)
			assert.Equal(t, tt.expected.AutojoinEnabled, result.AutojoinEnabled)
			assert.Equal(t, tt.expected.CharterURL, result.CharterURL)
			assert.Equal(t, tt.expected.LogoURL, result.LogoURL)
			assert.Equal(t, tt.expected.WebsiteURL, result.WebsiteURL)
			assert.Equal(t, tt.expected.RepositoryURL, result.RepositoryURL)
			assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			assert.Equal(t, tt.expected.AnnouncementDate, result.AnnouncementDate)
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)

			if tt.expected.FormationDate != nil {
				assert.Equal(t, tt.expected.FormationDate, result.FormationDate)
			}
		})
	}
}

func TestConvertToServiceProjectBase(t *testing.T) {
	now := time.Now()
	formationDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *ProjectBase
		expected *projsvc.ProjectBase
	}{
		{
			name: "complete project base",
			input: &ProjectBase{
				UID:             "test-uid",
				Slug:            "test-slug",
				Name:            "Test Project",
				Description:     "Test Description",
				Public:          true,
				ParentUID:       "parent-uid",
				Stage:           "incubating",
				Category:        "foundation",
				LegalEntityType: "LLC",
				LegalEntityName: "Test LLC",
				FundingModel:    []string{"donations", "grants"},
				FormationDate:   &formationDate,
				AutojoinEnabled: true,
				CharterURL:      "https://example.com/charter",
				LogoURL:         "https://example.com/logo.png",
				WebsiteURL:      "https://example.com",
				RepositoryURL:   "https://github.com/test/repo",
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
				Stage:           misc.StringPtr("incubating"),
				Category:        misc.StringPtr("foundation"),
				LegalEntityType: misc.StringPtr("LLC"),
				LegalEntityName: misc.StringPtr("Test LLC"),
				FundingModel:    []string{"donations", "grants"},
				FormationDate:   misc.StringPtr("2020-01-15"),
				AutojoinEnabled: misc.BoolPtr(true),
				CharterURL:      misc.StringPtr("https://example.com/charter"),
				LogoURL:         misc.StringPtr("https://example.com/logo.png"),
				WebsiteURL:      misc.StringPtr("https://example.com"),
				RepositoryURL:   misc.StringPtr("https://github.com/test/repo"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToServiceProjectBase(tt.input)

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.Slug, result.Slug)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Public, result.Public)
			assert.Equal(t, tt.expected.ParentUID, result.ParentUID)
			assert.Equal(t, tt.expected.Stage, result.Stage)
			assert.Equal(t, tt.expected.Category, result.Category)
			assert.Equal(t, tt.expected.LegalEntityType, result.LegalEntityType)
			assert.Equal(t, tt.expected.LegalEntityName, result.LegalEntityName)
			assert.Equal(t, tt.expected.FundingModel, result.FundingModel)
			assert.Equal(t, tt.expected.AutojoinEnabled, result.AutojoinEnabled)
			assert.Equal(t, tt.expected.CharterURL, result.CharterURL)
			assert.Equal(t, tt.expected.LogoURL, result.LogoURL)
			assert.Equal(t, tt.expected.WebsiteURL, result.WebsiteURL)
			assert.Equal(t, tt.expected.RepositoryURL, result.RepositoryURL)
			assert.Equal(t, tt.expected.FormationDate, result.FormationDate)
		})
	}
}

func TestConvertToServiceProjectSettings(t *testing.T) {
	now := time.Now()
	announcementDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    *ProjectSettings
		expected *projsvc.ProjectSettings
	}{
		{
			name: "complete project settings",
			input: &ProjectSettings{
				UID:              "test-uid",
				MissionStatement: "Our mission",
				AnnouncementDate: &announcementDate,
				Writers:          []string{"writer1", "writer2"},
				Auditors:         []string{"auditor1", "auditor2"},
				CreatedAt:        &now,
				UpdatedAt:        &now,
			},
			expected: &projsvc.ProjectSettings{
				UID:              misc.StringPtr("test-uid"),
				MissionStatement: misc.StringPtr("Our mission"),
				AnnouncementDate: misc.StringPtr("2023-06-01"),
				Writers:          []string{"writer1", "writer2"},
				Auditors:         []string{"auditor1", "auditor2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToServiceProjectSettings(tt.input)

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.UID, result.UID)
			assert.Equal(t, tt.expected.MissionStatement, result.MissionStatement)
			assert.Equal(t, tt.expected.AnnouncementDate, result.AnnouncementDate)
			assert.Equal(t, tt.expected.Writers, result.Writers)
			assert.Equal(t, tt.expected.Auditors, result.Auditors)
		})
	}
}
