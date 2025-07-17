// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	kvstore "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats-kv-store"
)

// ConvertToDBProject converts a project service project to a project database representation.
func ConvertToDBProject(project *projsvc.Project) *kvstore.ProjectDB {
	if project == nil {
		return new(kvstore.ProjectDB)
	}

	currentTime := time.Now()

	p := new(kvstore.ProjectDB)
	if project.ID != nil {
		p.UID = *project.ID
	}
	if project.Slug != nil {
		p.Slug = *project.Slug
	}
	if project.Name != nil {
		p.Name = *project.Name
	}
	if project.Description != nil {
		p.Description = *project.Description
	}
	if project.Public != nil {
		p.Public = *project.Public
	}
	if project.ParentUID != nil {
		p.ParentUID = *project.ParentUID
	}
	p.Auditors = project.Auditors
	p.Writers = project.Writers
	p.CreatedAt = currentTime
	p.UpdatedAt = currentTime

	return p
}

// ConvertToServiceProject converts a project database representation to a project service project.
func ConvertToServiceProject(p *kvstore.ProjectDB) *projsvc.Project {
	return &projsvc.Project{
		ID:          &p.UID,
		Slug:        &p.Slug,
		Name:        &p.Name,
		Description: &p.Description,
		Public:      &p.Public,
		ParentUID:   &p.ParentUID,
		Auditors:    p.Auditors,
		Writers:     p.Writers,
	}
}
