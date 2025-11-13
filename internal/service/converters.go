// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// ConvertToProjectFull merges ProjectBase and ProjectSettings into ProjectFull
func ConvertToProjectFull(base *models.ProjectBase, settings *models.ProjectSettings) *projsvc.ProjectFull {
	if base == nil {
		return nil
	}

	// Start with required fields
	full := &projsvc.ProjectFull{
		UID:  &base.UID,
		Slug: &base.Slug,
		// Public, IsFoundation and AutojoinEnabled are always included as they're booleans with meaningful zero values
		Public:          &base.Public,
		IsFoundation:    &base.IsFoundation,
		AutojoinEnabled: &base.AutojoinEnabled,
	}

	// Only set string fields if they're not empty
	if base.Name != "" {
		full.Name = &base.Name
	}
	if base.Description != "" {
		full.Description = &base.Description
	}
	if base.ParentUID != "" {
		full.ParentUID = &base.ParentUID
	}
	if base.Stage != "" {
		full.Stage = &base.Stage
	}
	if base.Category != "" {
		full.Category = &base.Category
	}
	if base.LegalEntityType != "" {
		full.LegalEntityType = &base.LegalEntityType
	}
	if base.LegalEntityName != "" {
		full.LegalEntityName = &base.LegalEntityName
	}
	if base.LegalParentUID != "" {
		full.LegalParentUID = &base.LegalParentUID
	}
	if base.CharterURL != "" {
		full.CharterURL = &base.CharterURL
	}
	if base.LogoURL != "" {
		full.LogoURL = &base.LogoURL
	}
	if base.WebsiteURL != "" {
		full.WebsiteURL = &base.WebsiteURL
	}
	if base.RepositoryURL != "" {
		full.RepositoryURL = &base.RepositoryURL
	}
	if base.EntityFormationDocumentURL != "" {
		full.EntityFormationDocumentURL = &base.EntityFormationDocumentURL
	}

	// Only set array fields if they're not empty
	if len(base.FundingModel) > 0 {
		full.FundingModel = base.FundingModel
	}
	// Handle base fields that are pointers
	if base.CreatedAt != nil {
		full.CreatedAt = misc.StringPtr(base.CreatedAt.Format(time.RFC3339))
	}
	if base.UpdatedAt != nil {
		full.UpdatedAt = misc.StringPtr(base.UpdatedAt.Format(time.RFC3339))
	}
	if base.EntityDissolutionDate != nil {
		full.EntityDissolutionDate = misc.StringPtr(base.EntityDissolutionDate.Format(time.DateOnly))
	}
	if base.FormationDate != nil {
		full.FormationDate = misc.StringPtr(base.FormationDate.Format(time.DateOnly))
	}

	// Add settings fields if available
	if settings != nil {
		// Only set string fields if they're not empty
		if settings.MissionStatement != "" {
			full.MissionStatement = &settings.MissionStatement
		}

		// Only set array fields if they're not empty
		if len(settings.Writers) > 0 {
			full.Writers = convertUsersToAPI(settings.Writers)
		}
		if len(settings.Auditors) > 0 {
			full.Auditors = convertUsersToAPI(settings.Auditors)
		}
		if len(settings.MeetingCoordinators) > 0 {
			full.MeetingCoordinators = convertUsersToAPI(settings.MeetingCoordinators)
		}

		// Handle settings fields that are pointers
		if settings.AnnouncementDate != nil {
			full.AnnouncementDate = misc.StringPtr(settings.AnnouncementDate.Format(time.DateOnly))
		}
	}

	return full
}

// ConvertToDBProjectBase converts a project service project to a project database representation.
func ConvertToDBProjectBase(project *projsvc.ProjectBase) (*models.ProjectBase, error) {
	if project == nil {
		return new(models.ProjectBase), nil
	}

	currentTime := time.Now().UTC()

	p := new(models.ProjectBase)
	if project.UID != nil {
		p.UID = *project.UID
	}
	if project.Slug != nil {
		p.Slug = *project.Slug
	}
	if project.Name != nil {
		p.Name = *project.Name
	}
	if project.Description != nil {
		p.Description = *project.Description
	}
	if project.Public != nil {
		p.Public = *project.Public
	}
	if project.IsFoundation != nil {
		p.IsFoundation = *project.IsFoundation
	}
	if project.ParentUID != nil {
		p.ParentUID = *project.ParentUID
	}
	if project.Stage != nil {
		p.Stage = *project.Stage
	}
	if project.Category != nil {
		p.Category = *project.Category
	}
	if project.LegalEntityType != nil {
		p.LegalEntityType = *project.LegalEntityType
	}
	if project.LegalEntityName != nil {
		p.LegalEntityName = *project.LegalEntityName
	}
	if project.LegalParentUID != nil {
		p.LegalParentUID = *project.LegalParentUID
	}
	if project.FundingModel != nil {
		p.FundingModel = project.FundingModel
	}
	if project.EntityDissolutionDate != nil {
		entityDissolutionDate, err := time.Parse(time.DateOnly, *project.EntityDissolutionDate)
		if err != nil {
			return nil, err
		}
		p.EntityDissolutionDate = &entityDissolutionDate
	}
	if project.EntityFormationDocumentURL != nil {
		p.EntityFormationDocumentURL = *project.EntityFormationDocumentURL
	}
	if project.FormationDate != nil {
		formationDate, err := time.Parse(time.DateOnly, *project.FormationDate)
		if err != nil {
			return nil, err
		}
		p.FormationDate = &formationDate
	}
	if project.AutojoinEnabled != nil {
		p.AutojoinEnabled = *project.AutojoinEnabled
	}
	if project.CharterURL != nil {
		p.CharterURL = *project.CharterURL
	}
	if project.LogoURL != nil {
		p.LogoURL = *project.LogoURL
	}
	if project.WebsiteURL != nil {
		p.WebsiteURL = *project.WebsiteURL
	}
	if project.RepositoryURL != nil {
		p.RepositoryURL = *project.RepositoryURL
	}
	if project.CreatedAt != nil {
		createdAt, err := time.Parse(time.RFC3339, *project.CreatedAt)
		if err != nil {
			return nil, err
		}
		p.CreatedAt = &createdAt
	} else {
		p.CreatedAt = &currentTime
	}
	if project.UpdatedAt != nil {
		updatedAt, err := time.Parse(time.RFC3339, *project.UpdatedAt)
		if err != nil {
			return nil, err
		}
		p.UpdatedAt = &updatedAt
	} else {
		p.UpdatedAt = &currentTime
	}

	return p, nil
}

// ConvertToServiceProjectBase converts a project database representation to a project service project.
func ConvertToServiceProjectBase(p *models.ProjectBase) *projsvc.ProjectBase {
	project := &projsvc.ProjectBase{
		UID:             &p.UID,
		Slug:            &p.Slug,
		Public:          &p.Public,
		IsFoundation:    &p.IsFoundation,
		AutojoinEnabled: &p.AutojoinEnabled,
	}

	// Only set string fields if they're not empty
	if p.Name != "" {
		project.Name = &p.Name
	}
	if p.Description != "" {
		project.Description = &p.Description
	}
	if p.ParentUID != "" {
		project.ParentUID = &p.ParentUID
	}
	if p.Stage != "" {
		project.Stage = &p.Stage
	}
	if p.Category != "" {
		project.Category = &p.Category
	}
	if p.LegalEntityType != "" {
		project.LegalEntityType = &p.LegalEntityType
	}
	if p.LegalEntityName != "" {
		project.LegalEntityName = &p.LegalEntityName
	}
	if p.LegalParentUID != "" {
		project.LegalParentUID = &p.LegalParentUID
	}
	if p.EntityFormationDocumentURL != "" {
		project.EntityFormationDocumentURL = &p.EntityFormationDocumentURL
	}
	if p.CharterURL != "" {
		project.CharterURL = &p.CharterURL
	}
	if p.LogoURL != "" {
		project.LogoURL = &p.LogoURL
	}
	if p.WebsiteURL != "" {
		project.WebsiteURL = &p.WebsiteURL
	}
	if p.RepositoryURL != "" {
		project.RepositoryURL = &p.RepositoryURL
	}

	// Only set array fields if they're not empty
	if len(p.FundingModel) > 0 {
		project.FundingModel = p.FundingModel
	}

	// Handle date fields that are pointers
	if p.EntityDissolutionDate != nil {
		project.EntityDissolutionDate = misc.StringPtr(p.EntityDissolutionDate.Format(time.DateOnly))
	}
	if p.FormationDate != nil {
		project.FormationDate = misc.StringPtr(p.FormationDate.Format(time.DateOnly))
	}
	if p.CreatedAt != nil {
		project.CreatedAt = misc.StringPtr(p.CreatedAt.Format(time.RFC3339))
	}
	if p.UpdatedAt != nil {
		project.UpdatedAt = misc.StringPtr(p.UpdatedAt.Format(time.RFC3339))
	}

	return project
}

// ConvertToDBProjectSettings converts a project settings service representation to a database representation.
func ConvertToDBProjectSettings(settings *projsvc.ProjectSettings) (*models.ProjectSettings, error) {
	if settings == nil {
		return new(models.ProjectSettings), nil
	}

	currentTime := time.Now().UTC()

	s := new(models.ProjectSettings)
	if settings.UID != nil {
		s.UID = *settings.UID
	}
	if settings.MissionStatement != nil {
		s.MissionStatement = *settings.MissionStatement
	}
	if settings.AnnouncementDate != nil {
		announcementDate, err := time.Parse(time.DateOnly, *settings.AnnouncementDate)
		if err != nil {
			return nil, err
		}
		s.AnnouncementDate = &announcementDate
	}
	if settings.Writers != nil {
		s.Writers = convertUsersFromAPI(settings.Writers)
	}
	if settings.Auditors != nil {
		s.Auditors = convertUsersFromAPI(settings.Auditors)
	}
	if settings.MeetingCoordinators != nil {
		s.MeetingCoordinators = convertUsersFromAPI(settings.MeetingCoordinators)
	}
	if settings.CreatedAt != nil {
		createdAt, err := time.Parse(time.RFC3339, *settings.CreatedAt)
		if err != nil {
			return nil, err
		}
		s.CreatedAt = &createdAt
	} else {
		s.CreatedAt = &currentTime
	}
	if settings.UpdatedAt != nil {
		updatedAt, err := time.Parse(time.RFC3339, *settings.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.UpdatedAt = &updatedAt
	} else {
		s.UpdatedAt = &currentTime
	}

	return s, nil
}

// ConvertToServiceProjectSettings converts a project settings database representation to a service representation.
func ConvertToServiceProjectSettings(s *models.ProjectSettings) *projsvc.ProjectSettings {
	settings := &projsvc.ProjectSettings{
		UID: &s.UID,
	}

	// Only set string fields if they're not empty
	if s.MissionStatement != "" {
		settings.MissionStatement = &s.MissionStatement
	}

	// Only set array fields if they're not empty
	if len(s.Writers) > 0 {
		settings.Writers = convertUsersToAPI(s.Writers)
	}
	if len(s.Auditors) > 0 {
		settings.Auditors = convertUsersToAPI(s.Auditors)
	}
	if len(s.MeetingCoordinators) > 0 {
		settings.MeetingCoordinators = convertUsersToAPI(s.MeetingCoordinators)
	}

	// Handle settings fields that are pointers
	if s.AnnouncementDate != nil {
		settings.AnnouncementDate = misc.StringPtr(s.AnnouncementDate.Format(time.DateOnly))
	}
	if s.CreatedAt != nil {
		settings.CreatedAt = misc.StringPtr(s.CreatedAt.Format(time.RFC3339))
	}
	if s.UpdatedAt != nil {
		settings.UpdatedAt = misc.StringPtr(s.UpdatedAt.Format(time.RFC3339))
	}

	return settings
}

// convertUsersToAPI converts domain UserInfo slice to API UserInfo slice
func convertUsersToAPI(users []models.UserInfo) []*projsvc.UserInfo {
	if len(users) == 0 {
		return nil
	}

	apiUsers := make([]*projsvc.UserInfo, len(users))
	for i := range users {
		user := users[i]
		apiUsers[i] = &projsvc.UserInfo{
			Name:     misc.StringPtr(user.Name),
			Email:    misc.StringPtr(user.Email),
			Username: misc.StringPtr(user.Username),
			Avatar:   misc.StringPtr(user.Avatar),
		}
	}
	return apiUsers
}

// convertUsersFromAPI converts API UserInfo slice to domain UserInfo slice
func convertUsersFromAPI(apiUsers []*projsvc.UserInfo) []models.UserInfo {
	if len(apiUsers) == 0 {
		return nil
	}

	users := make([]models.UserInfo, len(apiUsers))
	for i, apiUser := range apiUsers {
		if apiUser != nil {
			user := models.UserInfo{}
			if apiUser.Name != nil {
				user.Name = *apiUser.Name
			}
			if apiUser.Email != nil {
				user.Email = *apiUser.Email
			}
			if apiUser.Username != nil {
				user.Username = *apiUser.Username
			}
			if apiUser.Avatar != nil {
				user.Avatar = *apiUser.Avatar
			}
			users[i] = user
		}
	}
	return users
}

// extractUsernames extracts usernames from UserInfo slice for access control
func extractUsernames(users []models.UserInfo) []string {
	if len(users) == 0 {
		return nil
	}

	usernames := make([]string, len(users))
	for i, user := range users {
		usernames[i] = user.Username
	}
	return usernames
}

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
