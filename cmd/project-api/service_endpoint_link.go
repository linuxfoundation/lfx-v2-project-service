// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// CreateProjectLink creates a new project link.
func (s *ProjectsAPI) CreateProjectLink(ctx context.Context, payload *projsvc.CreateProjectLinkPayload) (*projsvc.ProjectLink, error) {
	link, err := s.service.CreateLink(ctx, payload.UID, payload.Name, payload.URL, misc.ValueOrZero(payload.Description), payload.FolderUID)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return service.ConvertToServiceLink(link), nil
}

// GetProjectLink gets a single project link.
func (s *ProjectsAPI) GetProjectLink(ctx context.Context, payload *projsvc.GetProjectLinkPayload) (*projsvc.GetProjectLinkResult, error) {
	link, etag, err := s.service.GetLink(ctx, payload.UID, payload.LinkUID)
	if err != nil {
		return nil, handleError(ctx, err)
	}

	return &projsvc.GetProjectLinkResult{
		Link: service.ConvertToServiceLink(link),
		Etag: &etag,
	}, nil
}

// DeleteProjectLink deletes a project link.
func (s *ProjectsAPI) DeleteProjectLink(ctx context.Context, payload *projsvc.DeleteProjectLinkPayload) error {
	if err := s.service.DeleteLink(ctx, payload.UID, payload.LinkUID, payload.IfMatch); err != nil {
		return handleError(ctx, err)
	}

	return nil
}
