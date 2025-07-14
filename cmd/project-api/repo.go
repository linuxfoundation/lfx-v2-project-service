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
	currentTime := time.Now()

	p := new(kvstore.ProjectDB)
	p.UID = *project.ID
	p.Slug = *project.Slug
	p.Name = *project.Name
	p.Description = *project.Description
	p.Managers = project.Managers
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
		Managers:    p.Managers,
	}
}
