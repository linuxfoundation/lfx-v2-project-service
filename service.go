// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
// The lfx-v2-project-service service.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/gen/project_service"
	"github.com/nats-io/nats.go/jetstream"

	"goa.design/goa/v3/security"
)

// ProjectsService implements the projsvc.Service interface
type ProjectsService struct {
	logger     *slog.Logger
	projectsKV INatsKeyValue
	natsConn   INatsConn
	auth       IJwtAuth
}

type contextID int

const (
	nonceSize                    = 24
	anonymousPrincipal           = `_anonymous`
	accessCheckSubject           = "dev.lfx.access_check.request"
	principalContextID contextID = iota
)

// TransactionBodyStub is used to decode the OpenSearch response's "source".
// **Ensure the fields here align to the relevant `SourceIncludes`
// parameters**.
type TransactionBodyStub struct {
	ObjectRef            string `json:"object_ref"`
	ObjectType           string `json:"object_type"`
	ObjectID             string `json:"object_id"`
	Public               bool   `json:"public"`
	AccessCheckObject    string `json:"access_check_object"`
	AccessCheckRelation  string `json:"access_check_relation"`
	HistoryCheckObject   string `json:"history_check_object"`
	HistoryCheckRelation string `json:"history_check_relation"`
	Data                 any    `json:"data"`
}

// IKeyValue is a NATS KV interface needed for the [ProjectsService].
type INatsKeyValue interface {
	ListKeys(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(context.Context, string, []byte) (uint64, error)
	Update(context.Context, string, []byte, uint64) (uint64, error)
	Delete(context.Context, string, ...jetstream.KVDeleteOpt) error
}

// INatsConn is a NATS connection interface needed for the [ProjectsService].
type INatsConn interface {
	IsConnected() bool
}

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	reqLogger := s.logger.With("method", "GetProjects")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload != nil && payload.PageToken != nil {
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
		reqLogger.With(errKey, err).Error("error getting project keys from NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV",
		}
	}

	projects := []*projsvc.Project{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, "slug/") {
			continue
		}

		entry, err := s.projectsKV.Get(ctx, key)
		if err != nil {
			reqLogger.With(errKey, err, "project_id", key).Error("error getting project from NATS KV")
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error getting project from NATS KV",
			}
		}

		projectDB := ProjectDB{}
		err = json.Unmarshal(entry.Value(), &projectDB)
		if err != nil {
			reqLogger.With(errKey, err, "project_id", key).Error("error unmarshalling project from NATS KV")
			return nil, &projsvc.InternalServerError{
				Code:    "500",
				Message: "error unmarshalling project from NATS KV",
			}
		}

		projects = append(projects, projectDB.ToProject())

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
		reqLogger.Error("NATS connection or KeyValue store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	projectDB := ProjectDB{}
	projectDB.FromProject(project)
	_, err := s.projectsKV.Put(ctx, fmt.Sprintf("slug/%s", projectDB.Slug), []byte(projectDB.UID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return nil, &projsvc.ConflictError{
				Code:    "409",
				Message: "project already exists",
			}
		}
		reqLogger.With(errKey, err, "project_id", projectDB.UID, "project_slug", projectDB.Slug).Error("error putting project UID mapping into NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error putting project UID mapping into NATS KV",
		}
	}

	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		reqLogger.With(errKey, err, "project_id", projectDB.UID).Error("error marshalling project into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Put(ctx, projectDB.UID, projectDBBytes)
	if err != nil {
		reqLogger.With(errKey, err, "project_id", projectDB.UID).Error("error putting project into NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error putting project into NATS KV",
		}
	}

	reqLogger.DebugContext(ctx, "returning created project", "project", project)

	return project, nil
}

// Get a single project.
func (s *ProjectsService) GetOneProject(ctx context.Context, payload *projsvc.GetOneProjectPayload) (*projsvc.Project, error) {
	reqLogger := s.logger.With("method", "GetOneProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KeyValue store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error getting project from NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV",
		}
	}

	projectDB := ProjectDB{}
	err = json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error unmarshalling project from NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error unmarshalling project from NATS KV",
		}
	}
	project := projectDB.ToProject()

	reqLogger.DebugContext(ctx, "returning project", "project", project)

	return project, nil

}

// Update a project.
func (s *ProjectsService) UpdateProject(ctx context.Context, payload *projsvc.UpdateProjectPayload) (*projsvc.Project, error) {
	reqLogger := s.logger.With("method", "UpdateProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	// return hardcoded response for now. Implement NATS later
	project := &projsvc.Project{
		ID:          payload.ProjectID,
		Slug:        &payload.Slug,
		Description: payload.Description,
		Name:        &payload.Name,
		Managers:    payload.Managers,
	}

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KeyValue store not initialized")
		return nil, &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error getting project from NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error getting project from NATS KV",
		}
	}
	revision := entry.Revision()

	projectDB := ProjectDB{}
	projectDB.FromProject(project)
	projectDBBytes, err := json.Marshal(projectDB)
	if err != nil {
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error marshalling project into JSON")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error marshalling project into JSON",
		}
	}
	_, err = s.projectsKV.Update(ctx, *payload.ProjectID, projectDBBytes, revision)
	if err != nil {
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error updating project in NATS KV")
		return nil, &projsvc.InternalServerError{
			Code:    "500",
			Message: "error updating project in NATS KV",
		}
	}

	reqLogger.DebugContext(ctx, "returning updated project", "project", project)

	return project, nil
}

// Delete a project.
func (s *ProjectsService) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	reqLogger := s.logger.With("method", "DeleteProject")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if s.natsConn == nil || s.projectsKV == nil {
		reqLogger.Error("NATS connection or KeyValue store not initialized")
		return &projsvc.ServiceUnavailableError{
			Code:    "503",
			Message: "service unavailable",
		}
	}

	entry, err := s.projectsKV.Get(ctx, *payload.ProjectID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return &projsvc.NotFoundError{
				Code:    "404",
				Message: "project not found",
			}
		}
	}
	revision := entry.Revision()

	err = s.projectsKV.Delete(ctx, *payload.ProjectID, jetstream.LastRevision(revision))
	if err != nil {
		reqLogger.With(errKey, err, "project_id", *payload.ProjectID).Error("error deleting project from NATS KV")
		return &projsvc.InternalServerError{
			Code:    "500",
			Message: "error deleting project from NATS KV",
		}
	}

	reqLogger.DebugContext(ctx, "deleted project", "project_id", payload.ProjectID)

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
	principal, err := s.auth.parsePrincipal(ctx, bearerToken, s.logger)
	if err != nil {
		return ctx, err
	}
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, principalContextID, principal), nil
}
