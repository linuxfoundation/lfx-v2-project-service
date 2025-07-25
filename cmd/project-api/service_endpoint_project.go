// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
)

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KeyValue store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	keysLister, err := s.kvStores.Projects.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project keys from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error listing project keys from NATS KV store")
	}

	// First, collect all project base data
	projectsBaseMap := make(map[string]*nats.ProjectBaseDB)
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "slug/") {
			continue
		}

		entry, err := s.kvStores.Projects.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err, "project_uid", key)
			return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
		}

		projectDB := nats.ProjectBaseDB{}
		err = json.Unmarshal(entry.Value(), &projectDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err, "project_uid", key)
			return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project from NATS KV store")
		}

		projectsBaseMap[key] = &projectDB
	}

	// Then, collect project settings data
	settingsKeysLister, err := s.kvStores.ProjectSettings.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project settings keys from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error listing project settings keys from NATS KV store")
	}

	projectsSettingsMap := make(map[string]*nats.ProjectSettingsDB)
	for key := range settingsKeysLister.Keys() {
		entry, err := s.kvStores.ProjectSettings.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project settings from NATS KV store", errKey, err, "project_uid", key)
			// Continue if settings not found - some projects might not have settings yet
			continue
		}

		projectSettingsDB := nats.ProjectSettingsDB{}
		err = json.Unmarshal(entry.Value(), &projectSettingsDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", errKey, err, "project_uid", key)
			// Continue if settings unmarshal fails
			continue
		}
		projectsSettingsMap[key] = &projectSettingsDB
	}

	// Combine base and settings to get the full project data
	projectsFull := []*projsvc.ProjectFull{}
	for uid, projectBase := range projectsBaseMap {
		projectSettings := projectsSettingsMap[uid] // May be nil if settings don't exist
		projectFull := ConvertToServiceProjectFull(projectBase, projectSettings)
		if projectFull != nil {
			projectsFull = append(projectsFull, projectFull)
		}
	}

	slog.DebugContext(ctx, "returning projects", "projects", projectsFull)

	return &projsvc.GetProjectsResult{
		Projects:     projectsFull,
		CacheControl: nil,
	}, nil

}

// Create a new project.
func (s *ProjectsService) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.ProjectFull, error) {
	id := uuid.NewString()
	project := &projsvc.ProjectBase{
		UID:                        &id,
		Slug:                       &payload.Slug,
		Description:                &payload.Description,
		Name:                       &payload.Name,
		Public:                     payload.Public,
		ParentUID:                  payload.ParentUID,
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
		UID:              &id,
		MissionStatement: payload.MissionStatement,
		AnnouncementDate: payload.AnnouncementDate,
		Writers:          payload.Writers,
		Auditors:         payload.Auditors,
	}

	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if project.ParentUID != nil && *project.ParentUID != "" {
		if _, err := uuid.Parse(*project.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "invalid parent UID")
		}
		if _, err := s.kvStores.Projects.Get(ctx, *project.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, createResponse(http.StatusBadRequest, "parent project not found")
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, createResponse(http.StatusInternalServerError, "error getting parent project from NATS KV store")
		}

	}

	// Check if the project slug is taken. No two projects should have the same slug.
	_, err := s.kvStores.Projects.Get(ctx, fmt.Sprintf("slug/%s", *project.Slug))
	if err == nil {
		// Slug already exists, return conflict
		slog.WarnContext(ctx, "project slug already exists", "slug", project.Slug)
		return nil, createResponse(http.StatusConflict, "project slug already exists")
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		// Some other error occurred while checking slug
		slog.ErrorContext(ctx, "error checking project slug availability", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error checking project slug availability")
	}

	// Store the project in the NATS KV store along with a mapping of the slug to the UID.
	projectDB, err := ConvertToDBProjectBase(project)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project to DB project", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error converting project to DB project")
	}

	slog.With("project_uid", projectDB.UID, "project_slug", projectDB.Slug)
	_, err = s.kvStores.Projects.Put(ctx, fmt.Sprintf("slug/%s", projectDB.Slug), []byte(projectDB.UID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			slog.WarnContext(ctx, "project already exists", errKey, err)
			return nil, createResponse(http.StatusConflict, "project already exists")
		}
		slog.ErrorContext(ctx, "error putting project UID mapping into NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error putting project UID mapping into NATS KV store")
	}

	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error marshalling project into JSON")
	}
	_, err = s.kvStores.Projects.Put(ctx, projectDB.UID, projectDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project into NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error putting project into NATS KV store")
	}

	// Store the project settings in the NATS KV store.
	projectSettingsDB, err := ConvertToDBProjectSettings(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project settings to DB project settings", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error converting project settings to DB project settings")
	}

	projectSettingsDBBytes, err := json.Marshal(projectSettingsDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error marshalling project into JSON")
	}
	_, err = s.kvStores.ProjectSettings.Put(ctx, projectSettingsDB.UID, projectSettingsDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project settings into NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error putting project settings into NATS KV store")
	}

	messageBuilder := nats.MessageBuilder{
		NatsConn:       s.natsConn,
		LfxEnvironment: s.lfxEnvironment,
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return messageBuilder.SendIndexProject(ctx, nats.ActionCreated, projectDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProject(ctx, projectDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendIndexProjectSettings(ctx, nats.ActionCreated, projectSettingsDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProjectSettings(ctx, projectSettingsDBBytes)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	// Create ProjectFull from base and settings
	projectFull := ConvertToServiceProjectFull(projectDB, projectSettingsDB)

	slog.DebugContext(ctx, "returning created project", "project", projectFull)

	return projectFull, nil
}

// Get a single project's base information.
func (s *ProjectsService) GetOneProjectBase(ctx context.Context, payload *projsvc.GetOneProjectBasePayload) (*projsvc.GetOneProjectBaseResult, error) {
	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, createResponse(http.StatusBadRequest, "project UID is required")
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	entry, err := s.kvStores.Projects.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	projectDB := nats.ProjectBaseDB{}
	err = json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project from NATS KV store")
	}
	project := ConvertToServiceProjectBase(&projectDB)

	// Store the revision in context for the custom encoder to use
	revision := entry.Revision()
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
	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, createResponse(http.StatusBadRequest, "project UID is required")
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Check if the project exists
	_, err := s.kvStores.Projects.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	entry, err := s.kvStores.ProjectSettings.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project settings from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project settings from NATS KV store")
	}

	projectSettingsDB := nats.ProjectSettingsDB{}
	err = json.Unmarshal(entry.Value(), &projectSettingsDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project settings from NATS KV store")
	}
	projectSettings := ConvertToServiceProjectSettings(&projectSettingsDB)

	// Store the revision in context for the custom encoder to use
	revision := entry.Revision()
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
	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, createResponse(http.StatusBadRequest, "project UID is required")
	}
	if payload.Etag == nil {
		slog.WarnContext(ctx, "ETag header is missing")
		return nil, createResponse(http.StatusBadRequest, "ETag header is missing")
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing ETag", errKey, err)
		return nil, createResponse(http.StatusBadRequest, "error parsing ETag header")
	}
	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))

	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Check if the project exists
	// TODO: have all calls to key-value stores be interfaced via a package instead of direct calls in the service code
	entryProject, err := s.kvStores.Projects.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	existingProjectDB := nats.ProjectBaseDB{}
	err = json.Unmarshal(entryProject.Value(), &existingProjectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project from NATS KV store")
	}

	// If the project slug is being changed, check if the requested slug is taken.
	// No two projects should have the same slug.
	if existingProjectDB.Slug != payload.Slug {
		_, err = s.kvStores.Projects.Get(ctx, fmt.Sprintf("slug/%s", payload.Slug))
		if err == nil {
			// Slug already exists, return conflict
			slog.WarnContext(ctx, "project slug already exists", "slug", payload.Slug)
			return nil, createResponse(http.StatusConflict, "project slug already exists")
		}
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			// Some other error occurred while checking slug
			slog.ErrorContext(ctx, "error checking project slug availability", errKey, err)
			return nil, createResponse(http.StatusInternalServerError, "error checking project slug availability")
		}
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if payload.ParentUID != nil && *payload.ParentUID != "" {
		if _, err := uuid.Parse(*payload.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "invalid parent UID")
		}
		if _, err := s.kvStores.Projects.Get(ctx, *payload.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, createResponse(http.StatusBadRequest, "parent project not found")
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, createResponse(http.StatusInternalServerError, "error getting parent project from NATS KV store")
		}
	}

	// Update the project in the NATS KV store
	project := &projsvc.ProjectBase{
		UID:                        payload.UID,
		Slug:                       &payload.Slug,
		Description:                &payload.Description,
		Name:                       &payload.Name,
		Public:                     payload.Public,
		ParentUID:                  payload.ParentUID,
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
	}
	projectDB, err := ConvertToDBProjectBase(project)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project to DB project", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error converting project to DB project")
	}
	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error marshalling project into JSON")
	}
	_, err = s.kvStores.Projects.Update(ctx, *payload.UID, projectDBBytes, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "etag header is invalid", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "etag header is invalid")
		}
		slog.ErrorContext(ctx, "error updating project in NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error updating project in NATS KV store")
	}

	messageBuilder := nats.MessageBuilder{
		NatsConn:       s.natsConn,
		LfxEnvironment: s.lfxEnvironment,
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return messageBuilder.SendIndexProject(ctx, nats.ActionUpdated, projectDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProject(ctx, projectDBBytes)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "returning updated project", "project", project)

	return project, nil
}

// Update a project's settings.
func (s *ProjectsService) UpdateProjectSettings(ctx context.Context, payload *projsvc.UpdateProjectSettingsPayload) (*projsvc.ProjectSettings, error) {
	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return nil, createResponse(http.StatusBadRequest, "project UID is required")
	}
	if payload.Etag == nil {
		slog.WarnContext(ctx, "ETag header is missing")
		return nil, createResponse(http.StatusBadRequest, "ETag header is missing")
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing ETag", errKey, err)
		return nil, createResponse(http.StatusBadRequest, "error parsing ETag header")
	}
	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))

	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Check if the project exists
	_, err = s.kvStores.Projects.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	// Update the project settings in the NATS KV store
	projectSettings := &projsvc.ProjectSettings{
		UID:              payload.UID,
		MissionStatement: payload.MissionStatement,
		AnnouncementDate: payload.AnnouncementDate,
		Writers:          payload.Writers,
		Auditors:         payload.Auditors,
	}
	projectSettingsDB, err := ConvertToDBProjectSettings(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error converting project settings to DB project settings", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error converting project settings to DB project settings")
	}
	projectSettingsDBBytes, err := json.Marshal(projectSettingsDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error marshalling project into JSON")
	}
	_, err = s.kvStores.ProjectSettings.Update(ctx, *payload.UID, projectSettingsDBBytes, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "etag header is invalid", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "etag header is invalid")
		}
		slog.ErrorContext(ctx, "error updating project in NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error updating project in NATS KV store")
	}

	messageBuilder := nats.MessageBuilder{
		NatsConn:       s.natsConn,
		LfxEnvironment: s.lfxEnvironment,
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return messageBuilder.SendIndexProjectSettings(ctx, nats.ActionUpdated, projectSettingsDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProjectSettings(ctx, projectSettingsDBBytes)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "returning updated project settings", "project_settings", projectSettings)

	return projectSettings, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	if payload == nil || payload.UID == nil {
		slog.WarnContext(ctx, "project UID is required")
		return createResponse(http.StatusBadRequest, "project UID is required")
	}
	if payload.Etag == nil {
		slog.WarnContext(ctx, "ETag header is missing")
		return createResponse(http.StatusBadRequest, "ETag header is missing")
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing ETag", errKey, err)
		return createResponse(http.StatusBadRequest, "error parsing ETag header")
	}

	ctx = log.AppendCtx(ctx, slog.String("project_uid", *payload.UID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	if s.natsConn == nil || s.kvStores.Projects == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Check if the project exists
	_, err = s.kvStores.Projects.Get(ctx, *payload.UID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return createResponse(http.StatusNotFound, "project not found")
		}
	}

	// Delete the project from the NATS KV store
	err = s.kvStores.Projects.Delete(ctx, *payload.UID, jetstream.LastRevision(revision))
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "etag header is invalid", errKey, err)
			return createResponse(http.StatusBadRequest, "etag header is invalid")
		}
		slog.ErrorContext(ctx, "error deleting project from NATS KV store", errKey, err)
		return createResponse(http.StatusInternalServerError, "error deleting project from NATS KV store")
	}

	messageBuilder := nats.MessageBuilder{
		NatsConn:       s.natsConn,
		LfxEnvironment: s.lfxEnvironment,
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return messageBuilder.SendIndexProject(ctx, nats.ActionDeleted, []byte(*payload.UID))
	})

	g.Go(func() error {
		return messageBuilder.SendDeleteAllAccessProject(ctx, []byte(*payload.UID))
	})

	g.Go(func() error {
		return messageBuilder.SendDeleteAllAccessProjectSettings(ctx, []byte(*payload.UID))
	})

	g.Go(func() error {
		return messageBuilder.SendIndexProjectSettings(ctx, nats.ActionDeleted, []byte(*payload.UID))
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "deleted project", "project_uid", *payload.UID)
	return nil
}
