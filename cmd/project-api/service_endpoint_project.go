// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// handleError converts domain errors to HTTP errors and logs client-facing failures.
func handleError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrServiceUnavailable):
		return createResponse(http.StatusServiceUnavailable, domain.ErrServiceUnavailable)
	case errors.Is(err, domain.ErrValidationFailed):
		slog.WarnContext(ctx, "request validation failed", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrValidationFailed)
	case errors.Is(err, domain.ErrRevisionMismatch):
		return createResponse(http.StatusConflict, domain.ErrRevisionMismatch)
	case errors.Is(err, domain.ErrInvalidParentProject):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrInvalidParentProject)
	case errors.Is(err, domain.ErrCannotDeleteNonCrowdfundingProject):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrCannotDeleteNonCrowdfundingProject)
	case errors.Is(err, domain.ErrArchivedRequiresDissolutionDate):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, domain.ErrArchivedRequiresDissolutionDate)
	case errors.Is(err, domain.ErrInvalidContentType), errors.Is(err, domain.ErrFileTooLarge):
		slog.WarnContext(ctx, "bad request", constants.ErrKey, err)
		return createResponse(http.StatusBadRequest, err)
	case errors.Is(err, domain.ErrProjectNotFound):
		return createResponse(http.StatusNotFound, domain.ErrProjectNotFound)
	case errors.Is(err, domain.ErrDocumentNotFound):
		return createResponse(http.StatusNotFound, domain.ErrDocumentNotFound)
	case errors.Is(err, domain.ErrLinkNotFound):
		return createResponse(http.StatusNotFound, domain.ErrLinkNotFound)
	case errors.Is(err, domain.ErrFolderNotFound):
		return createResponse(http.StatusNotFound, domain.ErrFolderNotFound)
	case errors.Is(err, domain.ErrProjectSlugExists):
		return createResponse(http.StatusConflict, domain.ErrProjectSlugExists)
	case errors.Is(err, domain.ErrDocumentNameExists):
		return createResponse(http.StatusConflict, domain.ErrDocumentNameExists)
	case errors.Is(err, domain.ErrFolderNameExists):
		return createResponse(http.StatusConflict, domain.ErrFolderNameExists)
	case errors.Is(err, domain.ErrFolderNotEmpty):
		return createResponse(http.StatusConflict, domain.ErrFolderNotEmpty)
	case errors.Is(err, domain.ErrInternal), errors.Is(err, domain.ErrUnmarshal):
		return createResponse(http.StatusInternalServerError, domain.ErrInternal)
	}
	return err
}

// GetProjects fetches all projects
func (s *ProjectsAPI) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	projects, err := s.service.GetProjects(ctx)
	if err != nil {
		return nil, handleError(ctx, err)
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
		return nil, handleError(ctx, err)
	}
	return project, nil
}

// GetOneProjectBase gets a single project's base information.
func (s *ProjectsAPI) GetOneProjectBase(ctx context.Context, payload *projsvc.GetOneProjectBasePayload) (*projsvc.GetOneProjectBaseResult, error) {
	project, err := s.service.GetOneProjectBase(ctx, payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return project, nil
}

// GetOneProjectSettings gets a single project's settings information.
func (s *ProjectsAPI) GetOneProjectSettings(ctx context.Context, payload *projsvc.GetOneProjectSettingsPayload) (*projsvc.GetOneProjectSettingsResult, error) {
	projectSettings, err := s.service.GetOneProjectSettings(ctx, payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return projectSettings, nil
}

// UpdateProjectBase updates a project's base information.
func (s *ProjectsAPI) UpdateProjectBase(ctx context.Context, payload *projsvc.UpdateProjectBasePayload) (*projsvc.ProjectBase, error) {
	updatedProject, err := s.service.UpdateProjectBase(ctx, payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return updatedProject, nil
}

// UpdateProjectSettings updates a project's settings.
func (s *ProjectsAPI) UpdateProjectSettings(ctx context.Context, payload *projsvc.UpdateProjectSettingsPayload) (*projsvc.ProjectSettings, error) {
	updatedProjectSettings, err := s.service.UpdateProjectSettings(ctx, payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return updatedProjectSettings, nil
}

// DeleteProject deletes a project.
func (s *ProjectsAPI) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	err := s.service.DeleteProject(ctx, payload)
	if err != nil {
		return handleError(ctx, err)
	}
	return nil
}

// ResolveProjectSlug resolves a project slug to its UID.
func (s *ProjectsAPI) ResolveProjectSlug(ctx context.Context, payload *projsvc.ResolveProjectSlugPayload) (*projsvc.ResolveProjectSlugResult, error) {
	result, err := s.service.ResolveProjectSlug(ctx, payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return result, nil
}
