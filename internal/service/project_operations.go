// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"golang.org/x/sync/errgroup"
)

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context) ([]*projsvc.ProjectFull, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	// Get all projects from the store
	projectsBase, projectsSettings, err := s.ProjectRepository.ListAllProjects(ctx)
	if err != nil {
		return nil, err
	}

	// Create maps for easy lookup
	projectsBaseMap := make(map[string]*models.ProjectBase)
	for _, proj := range projectsBase {
		projectsBaseMap[proj.UID] = proj
	}

	projectsSettingsMap := make(map[string]*models.ProjectSettings)
	for _, settings := range projectsSettings {
		projectsSettingsMap[settings.UID] = settings
	}

	// Combine base and settings to get the full project data
	projectsFull := []*projsvc.ProjectFull{}
	for uid, projectBase := range projectsBaseMap {
		projectSettings := projectsSettingsMap[uid] // May be nil if settings don't exist
		projectFull := ConvertToProjectFull(projectBase, projectSettings)
		if projectFull != nil {
			projectsFull = append(projectsFull, projectFull)
		}
	}

	slog.DebugContext(ctx, "returning projects", "projects", projectsFull)

	return projectsFull, nil
}

// CreateProject creates a new project
func (s *ProjectsService) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.ProjectFull, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	// Check if slug exists
	exists, err := s.ProjectRepository.ProjectSlugExists(ctx, payload.Slug)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if slug exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if exists {
		// Slug already exists
		return nil, domain.ErrProjectSlugExists
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if payload.ParentUID != "" {
		if _, err := uuid.Parse(payload.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", constants.ErrKey, err)
			return nil, domain.ErrValidationFailed
		}
		exists, err := s.ProjectRepository.ProjectExists(ctx, payload.ParentUID)
		if err != nil {
			slog.ErrorContext(ctx, "error checking if parent project exists", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
		if !exists {
			slog.ErrorContext(ctx, "parent project not found", constants.ErrKey, err)
			return nil, domain.ErrInvalidParentProject
		}
	}

	runSync := false
	if payload.XSync != nil {
		runSync = *payload.XSync
	}

	// Create the project and settings structs
	id := uuid.NewString()
	project := &projsvc.ProjectBase{
		UID:                        &id,
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
		FundingModel:               payload.FundingModel,
		EntityDissolutionDate:      payload.EntityDissolutionDate,
		EntityFormationDocumentURL: payload.EntityFormationDocumentURL,
		FormationDate:              payload.FormationDate,
		AutojoinEnabled:            payload.AutojoinEnabled,
		CharterURL:                 payload.CharterURL,
		LogoURL:                    payload.LogoURL,
		WebsiteURL:                 payload.WebsiteURL,
		RepositoryURL:              payload.RepositoryURL,
	}
	projectSettings := &projsvc.ProjectSettings{
		UID:                 &id,
		MissionStatement:    payload.MissionStatement,
		AnnouncementDate:    payload.AnnouncementDate,
		Writers:             payload.Writers,
		Auditors:            payload.Auditors,
		MeetingCoordinators: payload.MeetingCoordinators,
	}

	projectDB, err := ConvertToDBProjectBase(project)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project to DB project", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectSettingsDB, err := ConvertToDBProjectSettings(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project settings to DB project settings", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	// Create the project in the repository
	slog.With("project_uid", projectDB.UID, "project_slug", projectDB.Slug)
	err = s.ProjectRepository.CreateProject(ctx, projectDB, projectSettingsDB)
	if err != nil {
		if errors.Is(err, domain.ErrProjectSlugExists) {
			return nil, domain.ErrProjectSlugExists
		}
		return nil, domain.ErrInternal
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		msg := models.ProjectIndexerMessage{
			Action: models.ActionCreated,
			Data:   *projectDB,
			Tags:   projectDB.Tags(),
		}
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSubject, msg, runSync)
	})

	g.Go(func() error {
		msg := models.ProjectSettingsIndexerMessage{
			Action: models.ActionCreated,
			Data:   *projectSettingsDB,
			Tags:   projectSettingsDB.Tags(),
		}
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, msg, runSync)
	})

	g.Go(func() error {
		msg := buildFGAUpdateAccessMessage(projectDB, projectSettingsDB)
		return s.MessageBuilder.SendAccessMessage(ctx, constants.FGASyncUpdateAccessSubject, msg, runSync)
	})

	if err := g.Wait(); err != nil {
		return nil, domain.ErrInternal
	}

	projectFull := ConvertToProjectFull(projectDB, projectSettingsDB)

	slog.DebugContext(ctx, "returning created project", "project", projectFull)

	return projectFull, nil
}

func (s *ProjectsService) GetOneProjectBase(ctx context.Context, payload *projsvc.GetOneProjectBasePayload) (*projsvc.GetOneProjectBaseResult, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, domain.ErrValidationFailed
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))

	// Get project with revision from store
	projectDB, revision, err := s.ProjectRepository.GetProjectBaseWithRevision(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project from store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	project := ConvertToServiceProjectBase(projectDB)

	// Store the revision in context for the custom encoder to use
	revisionStr := strconv.FormatUint(revision, 10)
	ctx = context.WithValue(ctx, constants.ETagContextID, revisionStr)

	slog.DebugContext(ctx, "returning project", "project", project, "revision", revision)

	return &projsvc.GetOneProjectBaseResult{
		Project: project,
		Etag:    &revisionStr,
	}, nil
}

// Get a single project's settings information.
func (s *ProjectsService) GetOneProjectSettings(ctx context.Context, payload *projsvc.GetOneProjectSettingsPayload) (*projsvc.GetOneProjectSettingsResult, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, domain.ErrValidationFailed
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))

	// Check if the project exists
	exists, err := s.ProjectRepository.ProjectExists(ctx, *payload.UID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if !exists {
		slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
		return nil, domain.ErrProjectNotFound
	}

	// Get project settings with revision from store
	projectSettingsDB, revision, err := s.ProjectRepository.GetProjectSettingsWithRevision(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project settings not found", constants.ErrKey, err)
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project settings from store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectSettings := ConvertToServiceProjectSettings(projectSettingsDB)

	// Store the revision in context for the custom encoder to use
	revisionStr := strconv.FormatUint(revision, 10)
	ctx = context.WithValue(ctx, constants.ETagContextID, revisionStr)

	slog.DebugContext(ctx, "returning project settings", "project_settings", projectSettings, "revision", revision)

	return &projsvc.GetOneProjectSettingsResult{
		ProjectSettings: projectSettings,
		Etag:            &revisionStr,
	}, nil
}

// Update a project's base information.
func (s *ProjectsService) UpdateProjectBase(ctx context.Context, payload *projsvc.UpdateProjectBasePayload) (*projsvc.ProjectBase, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, domain.ErrValidationFailed
	}

	var revision uint64
	var err error
	if !s.Config.SkipEtagValidation {
		if payload.IfMatch == nil {
			slog.WarnContext(ctx, "If-Match header is missing")
			return nil, domain.ErrValidationFailed
		}
		revision, err = strconv.ParseUint(*payload.IfMatch, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "error parsing If-Match header", constants.ErrKey, err)
			return nil, domain.ErrValidationFailed
		}
	} else {
		// If skipping the Etag validation, we need to get the key revision from the store with a Get request.
		_, revision, err = s.ProjectRepository.GetProjectBaseWithRevision(ctx, *payload.UID)
		if err != nil {
			if errors.Is(err, domain.ErrProjectNotFound) {
				slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
				return nil, domain.ErrProjectNotFound
			}
			slog.ErrorContext(ctx, "error getting project from store", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	// Check if the project exists and use some of the existing project data for the update.
	existingProjectDB, err := s.ProjectRepository.GetProjectBase(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	// Check if the new slug is already taken
	if payload.Slug != existingProjectDB.Slug {
		newSlugExists, err := s.ProjectRepository.ProjectSlugExists(ctx, payload.Slug)
		if err != nil {
			slog.ErrorContext(ctx, "error checking if new slug exists", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
		if newSlugExists {
			// The slug is already taken
			return nil, domain.ErrProjectSlugExists
		}
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if payload.ParentUID != "" {
		if _, err := uuid.Parse(payload.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", constants.ErrKey, err)
			return nil, domain.ErrValidationFailed
		}
		exists, err := s.ProjectRepository.ProjectExists(ctx, payload.ParentUID)
		if err != nil {
			slog.ErrorContext(ctx, "error checking if parent project exists", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
		if !exists {
			slog.ErrorContext(ctx, "parent project not found", constants.ErrKey, err)
			return nil, domain.ErrProjectNotFound
		}
	}

	runSync := false
	if payload.XSync != nil {
		runSync = *payload.XSync
	}

	// Prepare the updated project
	currentTime := time.Now().UTC()
	project := &projsvc.ProjectBase{
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
		FundingModel:               payload.FundingModel,
		EntityDissolutionDate:      payload.EntityDissolutionDate,
		EntityFormationDocumentURL: payload.EntityFormationDocumentURL,
		AutojoinEnabled:            payload.AutojoinEnabled,
		FormationDate:              payload.FormationDate,
		CharterURL:                 payload.CharterURL,
		LogoURL:                    payload.LogoURL,
		WebsiteURL:                 payload.WebsiteURL,
		RepositoryURL:              payload.RepositoryURL,
		UpdatedAt:                  misc.StringPtr(currentTime.Format(time.RFC3339)),
	}
	if existingProjectDB.CreatedAt != nil {
		project.CreatedAt = misc.StringPtr(existingProjectDB.CreatedAt.Format(time.RFC3339))
	}

	projectDB, err := ConvertToDBProjectBase(project)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project to DB project", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	// Update the project in the repository
	err = s.ProjectRepository.UpdateProjectBase(ctx, projectDB, revision)
	if err != nil {
		if errors.Is(err, domain.ErrRevisionMismatch) {
			slog.WarnContext(ctx, "etag header is invalid", constants.ErrKey, err)
			return nil, domain.ErrRevisionMismatch
		}
		if errors.Is(err, domain.ErrInternal) {
			slog.ErrorContext(ctx, "error updating project in store", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
		return nil, domain.ErrInternal
	}

	projectSettingsDB, err := s.ProjectRepository.GetProjectSettings(ctx, *payload.UID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project settings from store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		msg := models.ProjectIndexerMessage{
			Action: models.ActionUpdated,
			Data:   *projectDB,
			Tags:   projectDB.Tags(),
		}
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSubject, msg, runSync)
	})

	g.Go(func() error {
		msg := buildFGAUpdateAccessMessage(projectDB, projectSettingsDB)
		return s.MessageBuilder.SendAccessMessage(ctx, constants.FGASyncUpdateAccessSubject, msg, runSync)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, domain.ErrInternal
	}

	slog.DebugContext(ctx, "returning updated project", "project", project)

	projectResp := ConvertToServiceProjectBase(projectDB)

	return projectResp, nil
}

// Update a project's settings.
func (s *ProjectsService) UpdateProjectSettings(ctx context.Context, payload *projsvc.UpdateProjectSettingsPayload) (*projsvc.ProjectSettings, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return nil, domain.ErrServiceUnavailable
	}

	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, domain.ErrValidationFailed
	}

	var revision uint64
	var err error
	if !s.Config.SkipEtagValidation {
		if payload.IfMatch == nil {
			slog.WarnContext(ctx, "If-Match header is missing")
			return nil, domain.ErrValidationFailed
		}
		revision, err = strconv.ParseUint(*payload.IfMatch, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "error parsing If-Match header", constants.ErrKey, err)
			return nil, domain.ErrValidationFailed
		}
	} else {
		// If skipping the Etag validation, we need to get the key revision from the store with a Get request.
		_, revision, err = s.ProjectRepository.GetProjectSettingsWithRevision(ctx, *payload.UID)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project settings from store", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	// Check if the project exists
	exists, err := s.ProjectRepository.ProjectExists(ctx, *payload.UID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}
	if !exists {
		slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
		return nil, domain.ErrProjectNotFound
	}

	// Get the existing project settings
	existingProjectSettingsDB, err := s.ProjectRepository.GetProjectSettings(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project settings not found", constants.ErrKey, err)
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project settings from store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	runSync := false
	if payload.XSync != nil {
		runSync = *payload.XSync
	}

	// Prepare the updated project settings
	currentTime := time.Now().UTC()
	projectSettings := &projsvc.ProjectSettings{
		UID:                 payload.UID,
		MissionStatement:    payload.MissionStatement,
		AnnouncementDate:    payload.AnnouncementDate,
		Writers:             payload.Writers,
		Auditors:            payload.Auditors,
		MeetingCoordinators: payload.MeetingCoordinators,
		UpdatedAt:           misc.StringPtr(currentTime.Format(time.RFC3339)),
	}
	if existingProjectSettingsDB.CreatedAt != nil {
		projectSettings.CreatedAt = misc.StringPtr(existingProjectSettingsDB.CreatedAt.Format(time.RFC3339))
	}

	projectSettingsDB, err := ConvertToDBProjectSettings(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project settings to DB project settings", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	// Update the project settings using the store
	err = s.ProjectRepository.UpdateProjectSettings(ctx, projectSettingsDB, revision)
	if err != nil {
		if errors.Is(err, domain.ErrRevisionMismatch) {
			slog.WarnContext(ctx, "etag header is invalid", constants.ErrKey, err)
			return nil, domain.ErrRevisionMismatch
		}
		if errors.Is(err, domain.ErrInternal) {
			slog.ErrorContext(ctx, "error updating project settings in store", constants.ErrKey, err)
			return nil, domain.ErrInternal
		}
		return nil, domain.ErrInternal
	}

	projectDB, err := s.ProjectRepository.GetProjectBase(ctx, *payload.UID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project from store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		msg := models.ProjectSettingsIndexerMessage{
			Action: models.ActionUpdated,
			Data:   *projectSettingsDB,
			Tags:   projectSettingsDB.Tags(),
		}
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, msg, runSync)
	})

	g.Go(func() error {
		msg := buildFGAUpdateAccessMessage(projectDB, projectSettingsDB)
		return s.MessageBuilder.SendAccessMessage(ctx, constants.FGASyncUpdateAccessSubject, msg, runSync)
	})

	g.Go(func() error {
		msg := models.ProjectSettingsUpdatedMessage{
			ProjectUID:  *payload.UID,
			OldSettings: *existingProjectSettingsDB,
			NewSettings: *projectSettingsDB,
		}
		return s.MessageBuilder.SendProjectEventMessage(ctx, constants.ProjectSettingsUpdatedSubject, msg)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, domain.ErrInternal
	}

	slog.DebugContext(ctx, "returning updated project settings", "project_settings", projectSettings)

	return projectSettings, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS connection or store not initialized")
		return domain.ErrServiceUnavailable
	}

	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return domain.ErrValidationFailed
	}

	var revision uint64
	var err error
	if !s.Config.SkipEtagValidation {
		if payload.IfMatch == nil {
			slog.WarnContext(ctx, "If-Match header is missing")
			return domain.ErrValidationFailed
		}
		revision, err = strconv.ParseUint(*payload.IfMatch, 10, 64)
		if err != nil {
			slog.ErrorContext(ctx, "error parsing If-Match header", constants.ErrKey, err)
			return domain.ErrValidationFailed
		}
	} else {
		// If skipping the Etag validation, we need to get the key revision from the store with a Get request.
		_, revision, err = s.ProjectRepository.GetProjectBaseWithRevision(ctx, *payload.UID)
		if err != nil {
			if errors.Is(err, domain.ErrProjectNotFound) {
				slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
				return domain.ErrProjectNotFound
			}
			slog.ErrorContext(ctx, "error getting project from store", constants.ErrKey, err)
			return domain.ErrInternal
		}
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	runSync := false
	if payload.XSync != nil {
		runSync = *payload.XSync
	}

	// Delete the project using the store
	err = s.ProjectRepository.DeleteProject(ctx, *payload.UID, revision)
	if err != nil {
		if errors.Is(err, domain.ErrRevisionMismatch) {
			slog.WarnContext(ctx, "etag header is invalid", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project not found", constants.ErrKey, err)
			return domain.ErrProjectNotFound
		}
		if errors.Is(err, domain.ErrInternal) {
			slog.ErrorContext(ctx, "error deleting project from store", constants.ErrKey, err)
			return domain.ErrInternal
		}
		return domain.ErrInternal
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSubject, *payload.UID, runSync)
	})

	g.Go(func() error {
		return s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, *payload.UID, runSync)
	})

	g.Go(func() error {
		msg := models.GenericFGAMessage{
			ObjectType: "project",
			Operation:  "delete_access",
			Data: models.DeleteAccessData{
				UID: *payload.UID,
			},
		}
		return s.MessageBuilder.SendAccessMessage(ctx, constants.FGASyncDeleteAccessSubject, msg, runSync)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return domain.ErrInternal
	}

	slog.DebugContext(ctx, "deleted project", "project_uid", *payload.UID)
	return nil
}
