// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// GetProjects fetches all projects
func (s *ProjectsAPI) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {
	projects, err := s.service.GetProjects(ctx)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	projectsFull := make([]*projsvc.ProjectFull, 0, len(projects))
	for _, project := range projects {
		projectsFull = append(projectsFull, convertToProjectFull(project.Base, project.Settings))
	}

	return &projsvc.GetProjectsResult{
		Projects:     projectsFull,
		CacheControl: nil,
	}, nil
}

// CreateProject creates a new project.
func (s *ProjectsAPI) CreateProject(ctx context.Context, payload *projsvc.CreateProjectPayload) (*projsvc.ProjectFull, error) {
	base, settings, err := createProjectInputs(payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	project, err := s.service.CreateProject(ctx, base, settings)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return convertToProjectFull(project.Base, project.Settings), nil
}

// GetOneProjectBase gets a single project's base information.
func (s *ProjectsAPI) GetOneProjectBase(ctx context.Context, payload *projsvc.GetOneProjectBasePayload) (*projsvc.GetOneProjectBaseResult, error) {
	project, etag, err := s.service.GetOneProjectBase(ctx, misc.ValueOrZero(payload.UID))
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return &projsvc.GetOneProjectBaseResult{
		Project: convertToServiceProjectBase(project),
		Etag:    &etag,
	}, nil
}

// GetOneProjectSettings gets a single project's settings information.
func (s *ProjectsAPI) GetOneProjectSettings(ctx context.Context, payload *projsvc.GetOneProjectSettingsPayload) (*projsvc.GetOneProjectSettingsResult, error) {
	projectSettings, etag, err := s.service.GetOneProjectSettings(ctx, misc.ValueOrZero(payload.UID))
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return &projsvc.GetOneProjectSettingsResult{
		ProjectSettings: convertToServiceProjectSettings(projectSettings),
		Etag:            &etag,
	}, nil
}

// UpdateProjectBase updates a project's base information.
func (s *ProjectsAPI) UpdateProjectBase(ctx context.Context, payload *projsvc.UpdateProjectBasePayload) (*projsvc.ProjectBase, error) {
	base, err := updateProjectBaseInput(payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	updatedProject, err := s.service.UpdateProjectBase(ctx, base, payload.IfMatch)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return convertToServiceProjectBase(updatedProject), nil
}

// UpdateProjectSettings updates a project's settings.
func (s *ProjectsAPI) UpdateProjectSettings(ctx context.Context, payload *projsvc.UpdateProjectSettingsPayload) (*projsvc.ProjectSettings, error) {
	settings, err := updateProjectSettingsInput(payload)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	updatedProjectSettings, err := s.service.UpdateProjectSettings(ctx, misc.ValueOrZero(payload.UID), settings, payload.IfMatch)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return convertToServiceProjectSettings(updatedProjectSettings), nil
}

// DeleteProject deletes a project.
func (s *ProjectsAPI) DeleteProject(ctx context.Context, payload *projsvc.DeleteProjectPayload) error {
	err := s.service.DeleteProject(ctx, misc.ValueOrZero(payload.UID), payload.IfMatch)
	if err != nil {
		return handleError(ctx, err)
	}
	return nil
}

// ResolveProjectSlug resolves a project slug to its UID.
func (s *ProjectsAPI) ResolveProjectSlug(ctx context.Context, payload *projsvc.ResolveProjectSlugPayload) (*projsvc.ResolveProjectSlugResult, error) {
	uid, err := s.service.ResolveProjectSlug(ctx, payload.Slug)
	if err != nil {
		return nil, handleError(ctx, err)
	}
	return &projsvc.ResolveProjectSlugResult{UID: uid}, nil
}
