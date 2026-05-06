// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

func toServiceLink(l *models.ProjectLink) *projsvc.ProjectLink {
	if l == nil {
		return nil
	}
	link := &projsvc.ProjectLink{
		UID:       &l.UID,
		FolderUID: l.FolderUID,
		Name:      &l.Name,
		URL:       &l.URL,
		CreatedAt: misc.StringPtr(l.CreatedAt.Format("2006-01-02T15:04:05Z07:00")),
		UpdatedAt: misc.StringPtr(l.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")),
	}
	if l.Description != "" {
		link.Description = &l.Description
	}
	if l.CreatedByUsername != "" {
		link.CreatedByUsername = &l.CreatedByUsername
	}
	return link
}

// CreateProjectLink creates a new project link.
func (s *ProjectsAPI) CreateProjectLink(ctx context.Context, payload *projsvc.CreateProjectLinkPayload) (*projsvc.ProjectLink, error) {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	link, err := s.service.CreateLink(ctx, payload.UID, payload.Name, payload.URL, nilStr(payload.Description), payload.FolderUID, xSync)
	if err != nil {
		return nil, handleError(err)
	}

	return toServiceLink(link), nil
}

// GetProjectLink gets a single project link.
func (s *ProjectsAPI) GetProjectLink(ctx context.Context, payload *projsvc.GetProjectLinkPayload) (*projsvc.GetProjectLinkResult, error) {
	link, etag, err := s.service.GetLink(ctx, payload.UID, payload.LinkUID)
	if err != nil {
		return nil, handleError(err)
	}

	return &projsvc.GetProjectLinkResult{
		Link: toServiceLink(link),
		Etag: &etag,
	}, nil
}

// ListProjectLinks lists all links for a project.
func (s *ProjectsAPI) ListProjectLinks(ctx context.Context, payload *projsvc.ListProjectLinksPayload) (*projsvc.ListProjectLinksResult, error) {
	links, err := s.service.ListLinks(ctx, payload.UID)
	if err != nil {
		return nil, handleError(err)
	}

	result := make([]*projsvc.ProjectLink, 0, len(links))
	for _, l := range links {
		result = append(result, toServiceLink(l))
	}

	return &projsvc.ListProjectLinksResult{Links: result}, nil
}

// DeleteProjectLink deletes a project link.
func (s *ProjectsAPI) DeleteProjectLink(ctx context.Context, payload *projsvc.DeleteProjectLinkPayload) error {
	xSync := false
	if payload.XSync != nil {
		xSync = *payload.XSync
	}

	if err := s.service.DeleteLink(ctx, payload.UID, payload.LinkUID, payload.IfMatch, xSync); err != nil {
		return handleError(err)
	}

	return nil
}

// nilStr returns empty string if pointer is nil, otherwise the value.
func nilStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
