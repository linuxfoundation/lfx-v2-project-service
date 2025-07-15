// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	kvstore "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats-kv-store"
	msg "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats-messaging"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
	"goa.design/goa/v3/security"
)

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	reqLogger := s.logger.With("method", "GetProjects")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload != nil && payload.PageToken != nil {
		reqLogger.With("page_token", *payload.PageToken).Warn("page token is not supported for projects query yet")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "page token is not supported for projects query yet",
		}
	}

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KeyValue store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	keysLister, err := s.projectsKV.ListKeys(ctx)
	if err != nil {
		reqLogger.With(errKey, err).Error("error listing project keys from NATS KV store")
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
			reqLogger.With(errKey, err, "project_id", key).Error("error getting project from NATS KV store")
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error getting project from NATS KV store",
			}
		}

		projectDB := kvstore.ProjectDB{}
		err = json.Unmarshal(entry.Value(), &projectDB)
		if err != nil {
			reqLogger.With(errKey, err, "project_id", key).Error("error unmarshalling project from NATS KV store")
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error unmarshalling project from NATS KV store",
			}
		}

		projects = append(projects, ConvertToServiceProject(&projectDB))

	}

	reqLogger.DebugContext(ctx, "returning projects", "projects", projects)

	return &projsvc.GetProjectsResult{
		Projects:     projects,
		PageToken:    nil,
		CacheControl: nil,
	}, nil

}

// Create a new project.
func (s *ProjectsService) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.Project, error) {
	reqLogger := s.logger.With("method", "CreateProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	id := uuid.NewString() // TODO: what type of ID are we using for the project resource?
	project := &projsvc.Project{
		ID:          &id,
		Slug:        &payload.Slug,
		Description: payload.Description,
		Name:        &payload.Name,
		Managers:    payload.Managers,
	}

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KV store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	projectDB := ConvertToDBProject(project)
	reqLogger = reqLogger.With("project_id", projectDB.UID, "project_slug", projectDB.Slug)
	_, err := s.projectsKV.Put(ctx, fmt.Sprintf("slug/%s", projectDB.Slug), []byte(projectDB.UID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			reqLogger.With(errKey, err).Warn("project already exists")
			return nil, &projsvc.ConflictError{
				Code:    "409",
				Message: "project already exists",
			}
		}
		reqLogger.With(errKey, err).Error("error putting project UID mapping into NATS KV store")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error putting project UID mapping into NATS KV store",
		}
	}

	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		reqLogger.With(errKey, err).Error("error marshalling project into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Put(ctx, projectDB.UID, projectDBBytes)
	if err != nil {
		reqLogger.With(errKey, err).Error("error putting project into NATS KV store")
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
		reqLogger.With(errKey, err).Error("error marshalling transaction into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for data indexing")

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.UpdateAccessProjectSubject)
	err = s.natsConn.Publish(subject, projectDBBytes)
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for access control updates")

	reqLogger.With("project", project).DebugContext(ctx, "returning created project")

	return project, nil
}

// Get a single project.
func (s *ProjectsService) GetOneProject(ctx context.Context, payload *projsvc.GetOneProjectPayload) (*projsvc.GetOneProjectResult, error) {
	reqLogger := s.logger.With("method", "GetOneProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload == nil || payload.ProjectID == nil {
		reqLogger.Warn("project ID is required")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}
	reqLogger = reqLogger.With("project_id", *payload.ProjectID)

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KV store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			reqLogger.With(errKey, err).Warn("project not found")
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		reqLogger.With(errKey, err).Error("error getting project from NATS KV store")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV store",
		}
	}

	projectDB := kvstore.ProjectDB{}
	err = json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		reqLogger.With(errKey, err).Error("error unmarshalling project from NATS KV store")
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

	reqLogger.DebugContext(ctx, "returning project", "project", project, "revision", revision)

	return &projsvc.GetOneProjectResult{
		Project: project,
		Etag:    &revisionStr,
	}, nil
}

// Update a project.
func (s *ProjectsService) UpdateProject(ctx context.Context, payload *projsvc.UpdateProjectPayload) (*projsvc.Project, error) {
	reqLogger := s.logger.With("method", "UpdateProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload == nil || payload.ProjectID == nil {
		reqLogger.Warn("project ID is required")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}
	if payload.Etag == nil {
		reqLogger.Warn("ETag header is missing")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "ETag header is missing",
		}
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		reqLogger.With(errKey, err).Error("error parsing ETag")
		return nil, &projsvc.BadRequestError{
			Code:    "400",
			Message: "error parsing ETag header",
		}
	}

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KV store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			reqLogger.With(errKey, err).Warn("project not found")
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		reqLogger.With(errKey, err).Error("error getting project from NATS KV store")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV store",
		}
	}

	// Update the project in the NATS KV store
	project := &projsvc.Project{
		ID:          payload.ProjectID,
		Slug:        &payload.Slug,
		Description: payload.Description,
		Name:        &payload.Name,
		Managers:    payload.Managers,
	}
	projectDB := ConvertToDBProject(project)
	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		reqLogger.With(errKey, err).Error("error marshalling project into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Update(ctx, *payload.ProjectID, projectDBBytes, revision)
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			reqLogger.With(errKey, err).Warn("etag header is invalid")
			return nil, &projsvc.BadRequestError{
				Code:    "400",
				Message: "etag header is invalid",
			}
		}
		reqLogger.With(errKey, err).Error("error updating project in NATS KV store")
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
		reqLogger.With(errKey, err).Error("error marshalling transaction into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for data indexing")

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.UpdateAccessProjectSubject)
	err = s.natsConn.Publish(subject, projectDBBytes)
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for access control updates")

	reqLogger.With("project", project).DebugContext(ctx, "returning updated project")

	return project, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	reqLogger := s.logger.With("method", "DeleteProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload == nil || payload.ProjectID == nil {
		reqLogger.Warn("project ID is required")
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "project ID is required",
		}
	}
	if payload.Etag == nil {
		reqLogger.Warn("ETag header is missing")
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "ETag header is missing",
		}
	}
	revision, err := strconv.ParseUint(*payload.Etag, 10, 64)
	if err != nil {
		reqLogger.With(errKey, err).Error("error parsing ETag")
		return &projsvc.BadRequestError{
			Code:    "400",
			Message: "error parsing ETag header",
		}
	}

	reqLogger = reqLogger.With("project_id", *payload.ProjectID).With("etag", revision)

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KV store not initialized")
		return &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	// Check if the project exists
	_, err = s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			reqLogger.With(errKey, err).Warn("project not found")
			return &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
	}

	// Delete the project from the NATS KV store
	err = s.projectsKV.Delete(ctx, *payload.ProjectID, jetstream.LastRevision(revision))
	if err != nil {
		if strings.Contains(err.Error(), "wrong last sequence") {
			reqLogger.With(errKey, err).Warn("etag header is invalid")
			return &projsvc.BadRequestError{
				Code:    "400",
				Message: "etag header is invalid",
			}
		}
		reqLogger.With(errKey, err).Error("error deleting project from NATS KV store")
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
		Data: payload.ProjectID,
	}

	transactionBytes, err := json.Marshal(transaction)
	if err != nil {
		reqLogger.With(errKey, err).Error("error marshalling transaction into JSON")
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling transaction into JSON",
		}
	}

	// Send the transaction to the NATS server for the data indexing.
	err = s.natsConn.Publish(subject, transactionBytes)
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for data indexing")

	// Send the transaction to the NATS server for the access control updates.
	subject = fmt.Sprintf("%s%s", s.lfxEnvironment, constants.DeleteAllAccessSubject)
	err = s.natsConn.Publish(subject, []byte(*payload.ProjectID))
	if err != nil {
		reqLogger.With(errKey, err, "subject", subject).Error("error sending transaction to NATS")
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error sending transaction to NATS",
		}
	}
	reqLogger.With("subject", subject).DebugContext(ctx, "sent transaction to NATS for access control deletion")

	reqLogger.With("project_id", *payload.ProjectID).DebugContext(ctx, "deleted project")

	return nil
}

// Readyz checks if the service is able to take inbound requests.
func (s *ProjectsService) Readyz(ctx context.Context) ([]byte, error) {
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
func (s *ProjectsService) Livez(context.Context) ([]byte, error) {
	// This always returns as long as the service is still running. As this
	// endpoint is expected to be used as a Kubernetes liveness check, this
	// service must likewise self-detect non-recoverable errors and
	// self-terminate.
	return []byte("OK\n"), nil
}

// JWTAuth implements Auther interface for the JWT security scheme.
func (s *ProjectsService) JWTAuth(ctx context.Context, bearerToken string, schema *security.JWTScheme) (context.Context, error) {
	// Parse the Heimdall-authorized principal from the token.
	// TODO: handle error
	principal, _ := s.auth.parsePrincipal(ctx, bearerToken, s.logger)
	// if err != nil {
	// 	return ctx, err
	// }
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}
