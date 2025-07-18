// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	kvstore "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats-kv-store"
	msg "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats-messaging"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
	"goa.design/goa/v3/security"
)

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KeyValue store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	keysLister, err := s.projectsKV.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project keys from NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error listing project keys from NATS KV store",
		}
	}

	projects := []*projsvc.Project{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "slug/") {
			continue
		}

		entry, err := s.projectsKV.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err, "project_id", key)
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error getting project from NATS KV store",
			}
		}

		projectDB := kvstore.ProjectDB{}
		err = json.Unmarshal(entry.Value(), &projectDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err, "project_id", key)
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error unmarshalling project from NATS KV store",
			}
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
	id := uuid.NewString() // TODO: what type of ID are we using for the project resource?
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
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if project.ParentUID != nil && *project.ParentUID != "" {
		if _, err := uuid.Parse(*project.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, &projsvc.BadRequestError{
				Code:    "400",
				Message: "invalid parent UID",
			}
		}
		if _, err := s.projectsKV.Get(ctx, *project.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, &projsvc.BadRequestError{
					Code:    "400",
					Message: "parent project not found",
				}
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error getting parent project from NATS KV store",
			}
		}

	}

	projectDB := ConvertToDBProject(project)
	slog.With("project_id", projectDB.UID, "project_slug", projectDB.Slug)
	_, err := s.projectsKV.Put(ctx, fmt.Sprintf("slug/%s", projectDB.Slug), []byte(projectDB.UID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			slog.WarnContext(ctx, "project already exists", errKey, err)
			return nil, &projsvc.ConflictError{
				Code:    "409",
				Message: "project already exists",
			}
		}
		slog.ErrorContext(ctx, "error putting project UID mapping into NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error putting project UID mapping into NATS KV store",
		}
	}

	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Put(ctx, projectDB.UID, projectDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project into NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error putting project into NATS KV store",
		}
	}

	// Prepare the transaction to be sent to the NATS server.
	subject := fmt.Sprintf("%s%s", s.lfxEnvironment, constants.IndexProjectSubject)
	transaction := msg.ProjectTransaction{
		Action:  msg.ActionCreated,
		Headers: map[string]string{},
		// Headers: map[string]string{
		// 	"authorization":  ctx.Value("authorization").(string),
		// 	"x-on-behalf-of": ctx.Value(constants.PrincipalContextID).(string),
		// },
		Data: projectDB,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling transaction into JSON", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for data indexing", "subject", subject)

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.UpdateAccessProjectSubject)
	err = s.natsConn.Publish(subject, projectDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for access control updates", "subject", subject)

	slog.DebugContext(ctx, "returning created project", "project", project)

	return project, nil
}

// Get a single project.
func (s *ProjectsService) GetOneProject(ctx context.Context, payload *projsvc.GetOneProjectPayload) (*projsvc.GetOneProjectResult, error) {
	if payload == nil || payload.ID == nil {
		slog.WarnContext(ctx, "project ID is required")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}

	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))
	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV store",
		}
	}

	projectDB := kvstore.ProjectDB{}
	err = json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error unmarshalling project from NATS KV store",
		}
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
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}
	if payload.Etag == nil {
		slog.WarnContext(ctx, "ETag header is missing")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "ETag header is missing",
		}
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing ETag", errKey, err)
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "error parsing ETag header",
		}
	}
	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))

	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	// Validate that the parent UID is a valid UUID and is an existing project UID.
	if payload.ParentUID != nil && *payload.ParentUID != "" {
		if _, err := uuid.Parse(*payload.ParentUID); err != nil {
			slog.ErrorContext(ctx, "invalid parent UID", errKey, err)
			return nil, &projsvc.BadRequestError{
				Code:    "400",
				Message: "invalid parent UID",
			}
		}
		if _, err := s.projectsKV.Get(ctx, *payload.ParentUID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				slog.ErrorContext(ctx, "parent project not found", errKey, err)
				return nil, &projsvc.BadRequestError{
					Code:    "400",
					Message: "parent project not found",
				}
			}
			slog.ErrorContext(ctx, "error getting parent project from NATS KV store", errKey, err)
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error getting parent project from NATS KV store",
			}
		}

	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV store",
		}
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
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Update(ctx, *payload.ID, projectDBBytes, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "etag header is invalid", errKey, err)
			return nil, &projsvc.BadRequestError{
				Code:    "400",
				Message: "etag header is invalid",
			}
		}
		slog.ErrorContext(ctx, "error updating project in NATS KV store", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error updating project in NATS KV store",
		}
	}

	// Prepare the transaction to be sent to the NATS server.
	subject := fmt.Sprintf("%s%s", s.lfxEnvironment, constants.IndexProjectSubject)
	transaction := msg.ProjectTransaction{
		Action:  msg.ActionUpdated,
		Headers: map[string]string{},
		// Headers: map[string]string{
		// 	"authorization":  ctx.Value("authorization").(string),
		// 	"x-on-behalf-of": ctx.Value(constants.PrincipalContextID).(string),
		// },
		Data: projectDB,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling transaction into JSON", errKey, err)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for data indexing", "subject", subject)

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.UpdateAccessProjectSubject)
	err = s.natsConn.Publish(subject, projectDBBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for access control updates", "subject", subject)

	slog.DebugContext(ctx, "returning updated project", "project", project)

	return project, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	if payload == nil || payload.ID == nil {
		slog.WarnContext(ctx, "project ID is required")
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}
	if payload.Etag == nil {
		slog.WarnContext(ctx, "ETag header is missing")
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "ETag header is missing",
		}
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing ETag", errKey, err)
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "error parsing ETag header",
		}
	}

	ctx = log.AppendCtx(ctx, slog.String("project_id", *payload.ID))
	ctx = log.AppendCtx(ctx, slog.String("etag", strconv.FormatUint(revision, 10)))

	if s.natsConn == nil || s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS connection or KV store not initialized")
		return &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			slog.WarnContext(ctx, "project not found", errKey, err)
			return &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
	}

	// Delete the project from the NATS KV store
	err = s.projectsKV.Delete(ctx, *payload.ID, jetstream.LastRevision(revision))
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			slog.WarnContext(ctx, "etag header is invalid", errKey, err)
			return &projsvc.BadRequestError{
				Code:    "400",
				Message: "etag header is invalid",
			}
		}
		slog.ErrorContext(ctx, "error deleting project from NATS KV store", errKey, err)
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error deleting project from NATS KV store",
		}
	}

	// Prepare the transaction to be sent to the NATS server.
	subject := fmt.Sprintf("%s%s", s.lfxEnvironment, constants.IndexProjectSubject)
	transaction := msg.ProjectTransaction{
		Action:  msg.ActionDeleted,
		Headers: map[string]string{},
		// Headers: map[string]string{
		// 	"authorization":  ctx.Value("authorization").(string),
		// 	"x-on-behalf-of": ctx.Value(constants.PrincipalContextID).(string),
		// },
		Data: payload.ID,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling transaction into JSON", errKey, err)
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for data indexing", "subject", subject)

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.DeleteAllAccessSubject)
	err = s.natsConn.Publish(subject, []byte(*payload.ID))
	if err != nil {
		slog.ErrorContext(ctx, "error sending transaction to NATS", errKey, err, "subject", subject)
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	slog.DebugContext(ctx, "sent transaction to NATS for access control deletion", "subject", subject)

	slog.DebugContext(ctx, "deleted project", "project_id", *payload.ID)

	return nil
}

// Readyz checks if the service is able to take inbound requests.
func (s *ProjectsService) Readyz(_ context.Context) ([]byte, error) {
	if s.natsConn == nil || s.projectsKV == nil {
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}
	if !s.natsConn.IsConnected() {
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "NATS connection not established",
		}
	}
	return []byte("OK\n"), nil
}

// Livez checks if the service is alive.
func (s *ProjectsService) Livez(_ context.Context) ([]byte, error) {
	// This always returns as long as the service is still running. As this
	// endpoint is expected to be used as a Kubernetes liveness check, this
	// service must likewise self-detect non-recoverable errors and
	// self-terminate.
	return []byte("OK\n"), nil
}

// JWTAuth implements Auther interface for the JWT security scheme.
func (s *ProjectsService) JWTAuth(ctx context.Context, bearerToken string, _ *security.JWTScheme) (context.Context, error) {
	// Parse the Heimdall-authorized principal from the token.
	principal, err := s.auth.parsePrincipal(ctx, bearerToken, slog.Default())
	if err != nil {
		return ctx, err
	}
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}
