// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
)

// handleError converts domain errors to HTTP errors.
func handleError(err error) error {
	switch err {
	case domain.ErrServiceUnavailable:
		return createResponse(http.StatusServiceUnavailable, domain.ErrServiceUnavailable)
	case domain.ErrValidationFailed:
		return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
	case domain.ErrRevisionMismatch:
		return createResponse(http.StatusBadRequest, domain.ErrRevisionMismatch)
	case domain.ErrProjectNotFound:
		return createResponse(http.StatusNotFound, domain.ErrProjectNotFound)
	case domain.ErrProjectSlugExists:
		return createResponse(http.StatusConflict, domain.ErrProjectSlugExists)
	case domain.ErrInternal, domain.ErrUnmarshal:
		return createResponse(http.StatusInternalServerError, domain.ErrInternal)
	}
	return err
}

// GetProjects fetches all projects
func (s *ProjectsAPI) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	projects, err := s.service.GetProjects(ctx)
	if err != nil {
		return nil, handleError(err)
	}

	return &projsvc.GetProjectsResult{
		Projects:     projects,
		CacheControl: nil,
	}, nil
}

// CreateProject creates a new project.
func (s *ProjectsAPI) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.ProjectFull, error) {
	project, err := s.service.CreateProject(ctx, payload)
	if err != nil {
		return nil, handleError(err)
	}
	return project, nil
}

// GetOneProjectBase gets a single project's base information.
func (s *ProjectsAPI) GetOneProjectBase(ctx context.Context, payload *projsvc.GetOneProjectBasePayload) (*projsvc.GetOneProjectBaseResult, error) {
	project, err := s.service.GetOneProjectBase(ctx, payload)
	if err != nil {
		return nil, handleError(err)
	}
	return project, nil
}

// GetOneProjectSettings gets a single project's settings information.
func (s *ProjectsAPI) GetOneProjectSettings(ctx context.Context, payload *projsvc.GetOneProjectSettingsPayload) (*projsvc.GetOneProjectSettingsResult, error) {
	projectSettings, err := s.service.GetOneProjectSettings(ctx, payload)
	if err != nil {
		return nil, handleError(err)
	}
	return projectSettings, nil
}

// UpdateProjectBase updates a project's base information.
func (s *ProjectsAPI) UpdateProjectBase(ctx context.Context, payload *projsvc.UpdateProjectBasePayload) (*projsvc.ProjectBase, error) {
	updatedProject, err := s.service.UpdateProjectBase(ctx, payload)
	if err != nil {
		return nil, handleError(err)
	}
	return updatedProject, nil
}

// UpdateProjectSettings updates a project's settings.
func (s *ProjectsAPI) UpdateProjectSettings(ctx context.Context, payload *projsvc.UpdateProjectSettingsPayload) (*projsvc.ProjectSettings, error) {
	updatedProjectSettings, err := s.service.UpdateProjectSettings(ctx, payload)
	if err != nil {
		return nil, handleError(err)
	}
	return updatedProjectSettings, nil
}

// DeleteProject deletes a project.
func (s *ProjectsAPI) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	err := s.service.DeleteProject(ctx, payload)
	if err != nil {
		return handleError(err)
	}
	return nil
}
