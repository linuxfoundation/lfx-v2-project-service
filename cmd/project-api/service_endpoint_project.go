// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
)

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
