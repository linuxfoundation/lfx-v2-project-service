// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// This file converts between the Goa API types and the types the service layer
// works with. The service layer never sees a Goa type, so everything that reads a
// request into domain terms or renders a domain model as a response lives here.

// convertToProjectFull merges ProjectBase and ProjectSettings into ProjectFull
func convertToProjectFull(base *models.ProjectBase, settings *models.ProjectSettings) *projsvc.ProjectFull {
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
	if base.Funding != "" {
		full.Funding = &base.Funding
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
	if base.ContactEmail != "" {
		full.ContactEmail = &base.ContactEmail
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
		if settings.ExecutiveDirector != nil {
			full.ExecutiveDirector = convertUserToAPI(settings.ExecutiveDirector)
		}
		if len(settings.MentorshipProgramAdmins) > 0 {
			full.MentorshipProgramAdmins = convertUsersToAPI(settings.MentorshipProgramAdmins)
		}
		if settings.ProgramManager != nil {
			full.ProgramManager = convertUserToAPI(settings.ProgramManager)
		}
		if settings.OpportunityOwner != nil {
			full.OpportunityOwner = convertUserToAPI(settings.OpportunityOwner)
		}

		// Handle settings fields that are pointers
		if settings.AnnouncementDate != nil {
			full.AnnouncementDate = misc.StringPtr(settings.AnnouncementDate.Format(time.DateOnly))
		}
	}

	return full
}

// convertToServiceProjectBase converts a project database representation to a project service project.
func convertToServiceProjectBase(p *models.ProjectBase) *projsvc.ProjectBase {
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
	if p.Funding != "" {
		project.Funding = &p.Funding
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
	if p.ContactEmail != "" {
		project.ContactEmail = &p.ContactEmail
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

// convertToServiceProjectSettings converts a project settings database representation to a service representation.
func convertToServiceProjectSettings(s *models.ProjectSettings) *projsvc.ProjectSettings {
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
	if s.ExecutiveDirector != nil {
		settings.ExecutiveDirector = convertUserToAPI(s.ExecutiveDirector)
	}
	if len(s.MentorshipProgramAdmins) > 0 {
		settings.MentorshipProgramAdmins = convertUsersToAPI(s.MentorshipProgramAdmins)
	}
	if s.ProgramManager != nil {
		settings.ProgramManager = convertUserToAPI(s.ProgramManager)
	}
	if s.OpportunityOwner != nil {
		settings.OpportunityOwner = convertUserToAPI(s.OpportunityOwner)
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
		apiUsers[i] = convertUserToAPI(&users[i])
	}
	return apiUsers
}

// convertAuditUserToAPI converts a domain UserInfo to a display-only AuditUserInfo for
// viewer-readable responses. Email is intentionally omitted to avoid exposing PII to
// anonymous callers on public projects.
func convertAuditUserToAPI(user *models.UserInfo) *projsvc.AuditUserInfo {
	if user == nil {
		return nil
	}
	return &projsvc.AuditUserInfo{
		Name:     misc.StringPtr(user.Name),
		Username: misc.StringPtr(user.Username),
		Avatar:   misc.StringPtr(user.Avatar),
	}
}

// convertUserToAPI converts a single domain UserInfo pointer to an API UserInfo pointer.
func convertUserToAPI(user *models.UserInfo) *projsvc.UserInfo {
	if user == nil {
		return nil
	}
	apiUser := &projsvc.UserInfo{
		Name:     misc.StringPtr(user.Name),
		Email:    misc.StringPtr(user.Email),
		Username: misc.StringPtr(user.Username),
		Avatar:   misc.StringPtr(user.Avatar),
	}
	if user.Invite != nil {
		inv := &projsvc.InviteInfo{
			UID:   misc.StringPtr(user.Invite.UID),
			Email: misc.StringPtr(user.Invite.Email),
		}
		if user.Invite.ExpiresAt != nil {
			s := user.Invite.ExpiresAt.UTC().Format(time.RFC3339)
			inv.ExpiresAt = &s
		}
		apiUser.Invite = inv
	}
	return apiUser
}

// convertToServiceDocument converts an internal ProjectDocument to the Goa API type.
func convertToServiceDocument(d *models.ProjectDocument) *projsvc.ProjectDocument {
	if d == nil {
		return nil
	}
	doc := &projsvc.ProjectDocument{
		UID:        &d.UID,
		ProjectUID: &d.ProjectUID,
		FolderUID:  d.FolderUID,
		Name:       &d.Name,
		FileName:   &d.FileName,
		FileSize:   &d.FileSize,
		CreatedBy:  convertAuditUserToAPI(d.CreatedBy),
		UpdatedBy:  convertAuditUserToAPI(d.UpdatedBy),
		CreatedAt:  misc.StringPtr(d.CreatedAt.Format(time.RFC3339)),
		UpdatedAt:  misc.StringPtr(d.UpdatedAt.Format(time.RFC3339)),
	}
	if d.Description != "" {
		doc.Description = &d.Description
	}
	if d.ContentType != "" {
		doc.ContentType = &d.ContentType
	}
	return doc
}

// convertToServiceLink converts an internal ProjectLink to the Goa API type.
func convertToServiceLink(l *models.ProjectLink) *projsvc.ProjectLink {
	if l == nil {
		return nil
	}
	link := &projsvc.ProjectLink{
		UID:        &l.UID,
		ProjectUID: &l.ProjectUID,
		FolderUID:  l.FolderUID,
		Name:       &l.Name,
		URL:        &l.URL,
		CreatedBy:  convertAuditUserToAPI(l.CreatedBy),
		UpdatedBy:  convertAuditUserToAPI(l.UpdatedBy),
		CreatedAt:  misc.StringPtr(l.CreatedAt.Format(time.RFC3339)),
		UpdatedAt:  misc.StringPtr(l.UpdatedAt.Format(time.RFC3339)),
	}
	if l.Description != "" {
		link.Description = &l.Description
	}
	return link
}

// convertToServiceFolder converts an internal ProjectFolder to the Goa API type.
func convertToServiceFolder(f *models.ProjectFolder) *projsvc.ProjectFolder {
	if f == nil {
		return nil
	}
	return &projsvc.ProjectFolder{
		UID:        &f.UID,
		ProjectUID: &f.ProjectUID,
		Name:       &f.Name,
		CreatedBy:  convertAuditUserToAPI(f.CreatedBy),
		UpdatedBy:  convertAuditUserToAPI(f.UpdatedBy),
		CreatedAt:  misc.StringPtr(f.CreatedAt.Format(time.RFC3339)),
		UpdatedAt:  misc.StringPtr(f.UpdatedAt.Format(time.RFC3339)),
	}
}

// parseOptionalDate reads an optional date string in layout. A nil, empty or
// whitespace-only value is treated as absent. field names the attribute in the
// validation error returned for an unparseable value.
func parseOptionalDate(field string, value *string, layout string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(layout, *value)
	if err != nil {
		return nil, domain.NewValidationError(fmt.Sprintf("%s is not a valid date", field))
	}
	return &parsed, nil
}

// projectBaseFromService converts the writable fields of an API project to a domain
// project. The service layer assigns the timestamps, so none are read here.
func projectBaseFromService(p *projsvc.ProjectBase) (*models.ProjectBase, error) {
	if p == nil {
		return new(models.ProjectBase), nil
	}
	dissolutionDate, err := parseOptionalDate("entity_dissolution_date", p.EntityDissolutionDate, time.DateOnly)
	if err != nil {
		return nil, err
	}
	formationDate, err := parseOptionalDate("formation_date", p.FormationDate, time.DateOnly)
	if err != nil {
		return nil, err
	}
	return &models.ProjectBase{
		UID:                        misc.ValueOrZero(p.UID),
		Slug:                       misc.ValueOrZero(p.Slug),
		Name:                       misc.ValueOrZero(p.Name),
		Description:                misc.ValueOrZero(p.Description),
		Public:                     misc.ValueOrZero(p.Public),
		IsFoundation:               misc.ValueOrZero(p.IsFoundation),
		ParentUID:                  misc.ValueOrZero(p.ParentUID),
		Stage:                      misc.ValueOrZero(p.Stage),
		Category:                   misc.ValueOrZero(p.Category),
		LegalEntityType:            misc.ValueOrZero(p.LegalEntityType),
		LegalEntityName:            misc.ValueOrZero(p.LegalEntityName),
		LegalParentUID:             misc.ValueOrZero(p.LegalParentUID),
		Funding:                    misc.ValueOrZero(p.Funding),
		FundingModel:               p.FundingModel,
		EntityDissolutionDate:      dissolutionDate,
		EntityFormationDocumentURL: misc.ValueOrZero(p.EntityFormationDocumentURL),
		FormationDate:              formationDate,
		AutojoinEnabled:            misc.ValueOrZero(p.AutojoinEnabled),
		CharterURL:                 misc.ValueOrZero(p.CharterURL),
		LogoURL:                    misc.ValueOrZero(p.LogoURL),
		WebsiteURL:                 misc.ValueOrZero(p.WebsiteURL),
		RepositoryURL:              misc.ValueOrZero(p.RepositoryURL),
		ContactEmail:               misc.ValueOrZero(p.ContactEmail),
	}, nil
}

// projectSettingsInputFromService converts the writable fields of API settings to
// the service layer's input. A nil role slice stays nil, so "left out" and
// "cleared" remain distinguishable.
func projectSettingsInputFromService(s *projsvc.ProjectSettings) (service.ProjectSettingsInput, error) {
	if s == nil {
		return service.ProjectSettingsInput{}, nil
	}
	announcementDate, err := parseOptionalDate("announcement_date", s.AnnouncementDate, time.DateOnly)
	if err != nil {
		return service.ProjectSettingsInput{}, err
	}
	return service.ProjectSettingsInput{
		MissionStatement:        misc.ValueOrZero(s.MissionStatement),
		AnnouncementDate:        announcementDate,
		Writers:                 userInputsFromService(s.Writers),
		Auditors:                userInputsFromService(s.Auditors),
		MeetingCoordinators:     userInputsFromService(s.MeetingCoordinators),
		ExecutiveDirector:       userInputFromService(s.ExecutiveDirector),
		MentorshipProgramAdmins: userInputsFromService(s.MentorshipProgramAdmins),
		ProgramManager:          userInputFromService(s.ProgramManager),
		OpportunityOwner:        userInputFromService(s.OpportunityOwner),
	}, nil
}

// userInputsFromService converts API users to service inputs, keeping a nil slice
// nil and a nil entry nil.
func userInputsFromService(users []*projsvc.UserInfo) []*service.UserInput {
	if users == nil {
		return nil
	}
	inputs := make([]*service.UserInput, len(users))
	for i, user := range users {
		inputs[i] = userInputFromService(user)
	}
	return inputs
}

// userInputFromService converts one API user to a service input. The invite is
// server-managed and is never read from a request.
func userInputFromService(user *projsvc.UserInfo) *service.UserInput {
	if user == nil {
		return nil
	}
	return &service.UserInput{
		Name:     user.Name,
		Email:    user.Email,
		Username: user.Username,
		Avatar:   user.Avatar,
	}
}

// createProjectInputs reads a create-project payload into the service layer's input.
func createProjectInputs(payload *projsvc.CreateProjectPayload) (*models.ProjectBase, service.ProjectSettingsInput, error) {
	base, err := projectBaseFromService(&projsvc.ProjectBase{
		Slug:                       &payload.Slug,
		Description:                &payload.Description,
		Name:                       &payload.Name,
		Public:                     payload.Public,
		IsFoundation:               payload.IsFoundation,
		ParentUID:                  &payload.ParentUID,
		Stage:                      payload.Stage,
		Category:                   payload.Category,
		LegalEntityType:            payload.LegalEntityType,
		LegalEntityName:            payload.LegalEntityName,
		LegalParentUID:             payload.LegalParentUID,
		Funding:                    payload.Funding,
		FundingModel:               payload.FundingModel,
		EntityDissolutionDate:      payload.EntityDissolutionDate,
		EntityFormationDocumentURL: payload.EntityFormationDocumentURL,
		FormationDate:              payload.FormationDate,
		AutojoinEnabled:            payload.AutojoinEnabled,
		CharterURL:                 payload.CharterURL,
		LogoURL:                    payload.LogoURL,
		WebsiteURL:                 payload.WebsiteURL,
		RepositoryURL:              payload.RepositoryURL,
		ContactEmail:               payload.ContactEmail,
	})
	if err != nil {
		return nil, service.ProjectSettingsInput{}, err
	}
	settings, err := projectSettingsInputFromService(&projsvc.ProjectSettings{
		MissionStatement:        payload.MissionStatement,
		AnnouncementDate:        payload.AnnouncementDate,
		Writers:                 payload.Writers,
		Auditors:                payload.Auditors,
		MeetingCoordinators:     payload.MeetingCoordinators,
		ExecutiveDirector:       payload.ExecutiveDirector,
		MentorshipProgramAdmins: payload.MentorshipProgramAdmins,
		ProgramManager:          payload.ProgramManager,
		OpportunityOwner:        payload.OpportunityOwner,
	})
	if err != nil {
		return nil, service.ProjectSettingsInput{}, err
	}
	return base, settings, nil
}

// updateProjectBaseInput reads an update-project-base payload into a domain project.
func updateProjectBaseInput(payload *projsvc.UpdateProjectBasePayload) (*models.ProjectBase, error) {
	return projectBaseFromService(&projsvc.ProjectBase{
		UID:                        payload.UID,
		Slug:                       &payload.Slug,
		Description:                &payload.Description,
		Name:                       &payload.Name,
		Public:                     payload.Public,
		IsFoundation:               payload.IsFoundation,
		ParentUID:                  &payload.ParentUID,
		Stage:                      payload.Stage,
		Category:                   payload.Category,
		LegalEntityType:            payload.LegalEntityType,
		LegalEntityName:            payload.LegalEntityName,
		LegalParentUID:             payload.LegalParentUID,
		Funding:                    payload.Funding,
		FundingModel:               payload.FundingModel,
		EntityDissolutionDate:      payload.EntityDissolutionDate,
		EntityFormationDocumentURL: payload.EntityFormationDocumentURL,
		AutojoinEnabled:            payload.AutojoinEnabled,
		FormationDate:              payload.FormationDate,
		CharterURL:                 payload.CharterURL,
		LogoURL:                    payload.LogoURL,
		WebsiteURL:                 payload.WebsiteURL,
		RepositoryURL:              payload.RepositoryURL,
		ContactEmail:               payload.ContactEmail,
	})
}

// updateProjectSettingsInput reads an update-project-settings payload into the
// service layer's input.
func updateProjectSettingsInput(payload *projsvc.UpdateProjectSettingsPayload) (service.ProjectSettingsInput, error) {
	return projectSettingsInputFromService(&projsvc.ProjectSettings{
		MissionStatement:        payload.MissionStatement,
		AnnouncementDate:        payload.AnnouncementDate,
		Writers:                 payload.Writers,
		Auditors:                payload.Auditors,
		MeetingCoordinators:     payload.MeetingCoordinators,
		ExecutiveDirector:       payload.ExecutiveDirector,
		MentorshipProgramAdmins: payload.MentorshipProgramAdmins,
		ProgramManager:          payload.ProgramManager,
		OpportunityOwner:        payload.OpportunityOwner,
	})
}
