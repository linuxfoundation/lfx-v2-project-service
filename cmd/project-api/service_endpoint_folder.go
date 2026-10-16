// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

// CreateProjectFolder creates a new project folder.
func (s *ProjectsAPI) CreateProjectFolder(ctx context.Context, payload *projsvc.CreateProjectFolderPayload) (*projsvc.ProjectFolder, error) {
	folder, err := s.service.CreateFolder(ctx, payload.UID, payload.Name)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return service.ConvertToServiceFolder(folder), nil
}

// GetProjectFolder gets a single project folder.
func (s *ProjectsAPI) GetProjectFolder(ctx context.Context, payload *projsvc.GetProjectFolderPayload) (*projsvc.GetProjectFolderResult, error) {
	folder, etag, err := s.service.GetFolder(ctx, payload.UID, payload.FolderUID)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return &projsvc.GetProjectFolderResult{
		Folder: service.ConvertToServiceFolder(folder),
		Etag:   &etag,
	}, nil
}

// DeleteProjectFolder deletes a project folder.
func (s *ProjectsAPI) DeleteProjectFolder(ctx context.Context, payload *projsvc.DeleteProjectFolderPayload) error {
	if err := s.service.DeleteFolder(ctx, payload.UID, payload.FolderUID, payload.IfMatch); err != nil {
		return handleError(ctx, err)
	}

	return nil
}
