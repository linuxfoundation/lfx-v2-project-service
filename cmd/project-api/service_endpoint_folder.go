// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func toServiceFolder(f *models.ProjectFolder) *projsvc.ProjectFolder {
	if f == nil {
		return nil
	}
	folder := &projsvc.ProjectFolder{
		UID:        &f.UID,
		ProjectUID: &f.ProjectUID,
		Name:       &f.Name,
		CreatedAt:  misc.StringPtr(f.CreatedAt.Format("2006-01-02T15:04:05Z07:00")),
		UpdatedAt:  misc.StringPtr(f.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")),
	}
	if f.CreatedByUsername != "" {
		folder.CreatedByUsername = &f.CreatedByUsername
	}
	return folder
}

// CreateProjectFolder creates a new project folder.
func (s *ProjectsAPI) CreateProjectFolder(ctx context.Context, payload *projsvc.CreateProjectFolderPayload) (*projsvc.ProjectFolder, error) {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	folder, err := s.service.CreateFolder(ctx, payload.UID, payload.Name, xSync)
	if err != nil {
		return nil, handleError(err)
	}

	return toServiceFolder(folder), nil
}

// GetProjectFolder gets a single project folder.
func (s *ProjectsAPI) GetProjectFolder(ctx context.Context, payload *projsvc.GetProjectFolderPayload) (*projsvc.GetProjectFolderResult, error) {
	folder, etag, err := s.service.GetFolder(ctx, payload.UID, payload.FolderUID)
	if err != nil {
		return nil, handleError(err)
	}

	return &projsvc.GetProjectFolderResult{
		Folder: toServiceFolder(folder),
		Etag:   &etag,
	}, nil
}

// DeleteProjectFolder deletes a project folder.
func (s *ProjectsAPI) DeleteProjectFolder(ctx context.Context, payload *projsvc.DeleteProjectFolderPayload) error {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	if err := s.service.DeleteFolder(ctx, payload.UID, payload.FolderUID, payload.IfMatch, xSync); err != nil {
		return handleError(err)
	}

	return nil
}
