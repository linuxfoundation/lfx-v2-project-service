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
	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KeyValue store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	keysLister, err := s.projectsKV.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project keys from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error listing project keys from NATS KV store")
	}

	projects := []*projsvc.Project{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "slug/") {
			continue
		}

		entry, err := s.projectsKV.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err, "project_id", key)
			return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
		}

		projectDB := nats.ProjectDB{}
		err = json.Unmarshal(entry.Value(), &projectDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err, "project_id", key)
			return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project from NATS KV store")
		}

		projects = append(projects, ConvertToServiceProject(&projectDB))

	}

	slog.DebugContext(ctx, "returning projects", "projects", projects)

	return &projsvc.GetProjectsResult{
		Projects:     projects,
		CacheControl: nil,
	}, nil

}

// Create a new project.
func (s *ProjectsService) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.Project, error) {
	id := uuid.NewString()
	project := &projsvc.Project{
		ID:          &id,
		Slug:        &payload.Slug,
		Description: &payload.Description,
		Name:        &payload.Name,
		Public:      payload.Public,
		ParentUID:   payload.ParentUID,
		Auditors:    payload.Auditors,
		Writers:     payload.Writers,
	}

	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if project.ParentUID != nil && *project.ParentUID != "" {
		if _, err := uuid.Parse(*project.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "invalid parent UID")
		}
		if _, err := s.projectsKV.Get(ctx, *project.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, createResponse(http.StatusBadRequest, "parent project not found")
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, createResponse(http.StatusInternalServerError, "error getting parent project from NATS KV store")
		}

	}

	projectDB := ConvertToDBProject(project)
	slog.With("project_id", projectDB.UID, "project_slug", projectDB.Slug)
	_, err := s.projectsKV.Put(ctx, fmt.Sprintf("slug/%s", projectDB.Slug), []byte(projectDB.UID))
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
	_, err = s.projectsKV.Put(ctx, projectDB.UID, projectDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project into NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error putting project into NATS KV store")
	}

	messageBuilder := nats.MessageBuilder{
		NatsConn:       s.natsConn,
		LfxEnvironment: s.lfxEnvironment,
	}

	g := new(errgroup.Group)
	g.Go(func() error {
		return messageBuilder.SendIndexProjectTransaction(ctx, nats.ActionCreated, projectDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProjectTransaction(ctx, projectDBBytes)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "returning created project", "project", project)

	return project, nil
}

// Get a single project.
func (s *ProjectsService) GetOneProject(ctx context.Context, payload *projsvc.GetOneProjectPayload) (*projsvc.GetOneProjectResult, error) {
	if payload == nil || payload.ID == nil {
		slog.WarnContext(ctx, "project ID is required")
		return nil, createResponse(http.StatusBadRequest, "project ID is required")
	}

	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))
	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	projectDB := nats.ProjectDB{}
	err = json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error unmarshalling project from NATS KV store")
	}
	project := ConvertToServiceProject(&projectDB)

	// Store the revision in context for the custom encoder to use
	revision := entry.Revision()
	revisionStr := strconv.FormatUint(revision, 10)
	ctx = context.WithValue(ctx, constants.ETagContextID, revisionStr)

	slog.DebugContext(ctx, "returning project", "project", project, "revision", revision)

	return &projsvc.GetOneProjectResult{
		Project: project,
		Etag:    &revisionStr,
	}, nil
}

// Update a project.
func (s *ProjectsService) UpdateProject(ctx context.Context, payload *projsvc.UpdateProjectPayload) (*projsvc.Project, error) {
	if payload == nil || payload.ID == nil {
		slog.WarnContext(ctx, "project ID is required")
		return nil, createResponse(http.StatusBadRequest, "project ID is required")
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
	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))

	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if payload.ParentUID != nil && *payload.ParentUID != "" {
		if _, err := uuid.Parse(*payload.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, createResponse(http.StatusBadRequest, "invalid parent UID")
		}
		if _, err := s.projectsKV.Get(ctx, *payload.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, createResponse(http.StatusBadRequest, "parent project not found")
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, createResponse(http.StatusInternalServerError, "error getting parent project from NATS KV store")
		}

	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, createResponse(http.StatusNotFound, "project not found")
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error getting project from NATS KV store")
	}

	// Update the project in the NATS KV store
	project := &projsvc.Project{
		ID:          payload.ID,
		Slug:        &payload.Slug,
		Description: &payload.Description,
		Name:        &payload.Name,
		Public:      payload.Public,
		ParentUID:   payload.ParentUID,
		Auditors:    payload.Auditors,
		Writers:     payload.Writers,
	}
	projectDB := ConvertToDBProject(project)
	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, createResponse(http.StatusInternalServerError, "error marshalling project into JSON")
	}
	_, err = s.projectsKV.Update(ctx, *payload.ID, projectDBBytes, revision)
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
		return messageBuilder.SendIndexProjectTransaction(ctx, nats.ActionUpdated, projectDBBytes)
	})

	g.Go(func() error {
		return messageBuilder.SendUpdateAccessProjectTransaction(ctx, projectDBBytes)
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return nil, createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "returning updated project", "project", project)

	return project, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	if payload == nil || payload.ID == nil {
		slog.WarnContext(ctx, "project ID is required")
		return createResponse(http.StatusBadRequest, "project ID is required")
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

	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return createResponse(http.StatusServiceUnavailable, "service unavailable")
	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return createResponse(http.StatusNotFound, "project not found")
		}
	}

	// Delete the project from the NATS KV store
	err = s.projectsKV.Delete(ctx, *payload.ID, jetstream.LastRevision(revision))
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
		return messageBuilder.SendIndexProjectTransaction(ctx, nats.ActionDeleted, []byte(*payload.ID))
	})

	g.Go(func() error {
		return messageBuilder.SendDeleteAllAccessProjectTransaction(ctx, []byte(*payload.ID))
	})

	if err := g.Wait(); err != nil {
		// Return the first error from the goroutines.
		return createResponse(http.StatusInternalServerError, fmt.Sprintf("error sending transactions to NATS: %s", err.Error()))
	}

	slog.DebugContext(ctx, "deleted project", "project_id", *payload.ID)
	return nil
}
