// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// ConvertToServiceProjectFull merges ProjectBase and ProjectSettings into ProjectFull
func ConvertToServiceProjectFull(base *nats.ProjectBaseDB, settings *nats.ProjectSettingsDB) *projsvc.ProjectFull {
	if base == nil {
		return nil
	}

	// Start with base fields
	full := &projsvc.ProjectFull{
		UID:                        &base.UID,
		Slug:                       &base.Slug,
		Name:                       &base.Name,
		Description:                &base.Description,
		Public:                     &base.Public,
		ParentUID:                  &base.ParentUID,
		Stage:                      &base.Stage,
		Category:                   &base.Category,
		LegalEntityType:            &base.LegalEntityType,
		LegalEntityName:            &base.LegalEntityName,
		LegalParentUID:             &base.LegalParentUID,
		FundingModel:               base.FundingModel,
		CharterURL:                 &base.CharterURL,
		LogoURL:                    &base.LogoURL,
		WebsiteURL:                 &base.WebsiteURL,
		RepositoryURL:              &base.RepositoryURL,
		EntityFormationDocumentURL: &base.EntityFormationDocumentURL,
		AutojoinEnabled:            &base.AutojoinEnabled,
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
		full.MissionStatement = &settings.MissionStatement
		full.Writers = settings.Writers
		full.Auditors = settings.Auditors

		// Handle settings fields that are pointers
		if settings.AnnouncementDate != nil {
			full.AnnouncementDate = misc.StringPtr(settings.AnnouncementDate.Format(time.DateOnly))
		}
	}

	return full
}

// ConvertToDBProjectBase converts a project service project to a project database representation.
func ConvertToDBProjectBase(project *projsvc.ProjectBase) (*nats.ProjectBaseDB, error) {
	if project == nil {
		return new(nats.ProjectBaseDB), nil
	}

	currentTime := time.Now().UTC()

	p := new(nats.ProjectBaseDB)
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
	p.CreatedAt = &currentTime
	p.UpdatedAt = &currentTime

	return p, nil
}

// ConvertToServiceProject converts a project database representation to a project service project.
func ConvertToServiceProjectBase(p *nats.ProjectBaseDB) *projsvc.ProjectBase {
	project := &projsvc.ProjectBase{
		UID:                        &p.UID,
		Slug:                       &p.Slug,
		Name:                       &p.Name,
		Description:                &p.Description,
		Public:                     &p.Public,
		ParentUID:                  &p.ParentUID,
		Stage:                      &p.Stage,
		Category:                   &p.Category,
		LegalEntityType:            &p.LegalEntityType,
		LegalEntityName:            &p.LegalEntityName,
		LegalParentUID:             &p.LegalParentUID,
		FundingModel:               p.FundingModel,
		EntityFormationDocumentURL: &p.EntityFormationDocumentURL,
		AutojoinEnabled:            &p.AutojoinEnabled,
		CharterURL:                 &p.CharterURL,
		LogoURL:                    &p.LogoURL,
		WebsiteURL:                 &p.WebsiteURL,
		RepositoryURL:              &p.RepositoryURL,
	}

	// Handle base fields that are pointers
	if p.EntityDissolutionDate != nil {
		project.EntityDissolutionDate = misc.StringPtr(p.EntityDissolutionDate.Format(time.DateOnly))
	}
	if p.FormationDate != nil {
		project.FormationDate = misc.StringPtr(p.FormationDate.Format(time.DateOnly))
	}

	return project
}

// ConvertToDBProjectSettings converts a project settings service representation to a database representation.
func ConvertToDBProjectSettings(settings *projsvc.ProjectSettings) (*nats.ProjectSettingsDB, error) {
	if settings == nil {
		return new(nats.ProjectSettingsDB), nil
	}

	currentTime := time.Now().UTC()

	s := new(nats.ProjectSettingsDB)
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
		s.Writers = settings.Writers
	}
	if settings.Auditors != nil {
		s.Auditors = settings.Auditors
	}
	s.CreatedAt = &currentTime
	s.UpdatedAt = &currentTime

	return s, nil
}

// ConvertToServiceProjectSettings converts a project settings database representation to a service representation.
func ConvertToServiceProjectSettings(s *nats.ProjectSettingsDB) *projsvc.ProjectSettings {
	settings := &projsvc.ProjectSettings{
		UID:              &s.UID,
		MissionStatement: &s.MissionStatement,
		Writers:          s.Writers,
		Auditors:         s.Auditors,
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
