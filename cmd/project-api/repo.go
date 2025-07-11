// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
// The project service.
package main

import (
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
)

// ProjectDB is the key-value store representation of a project.
type ProjectDB struct {
	UID         string    `json:"uid"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Managers    []string  `json:"managers"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FromProject converts a project service project to a project database representation.
func (p *ProjectDB) FromProject(project *projsvc.Project) {
	currentTime := time.Now()

	p.UID = *project.ID
	p.Slug = *project.Slug
	p.Name = *project.Name
	p.Description = *project.Description
	p.Managers = project.Managers
	p.CreatedAt = currentTime
	p.UpdatedAt = currentTime
}

// ToProject converts a project database representation to a project service project.
func (p *ProjectDB) ToProject() *projsvc.Project {
	return &projsvc.Project{
		ID:          &p.UID,
		Slug:        &p.Slug,
		Name:        &p.Name,
		Description: &p.Description,
		Managers:    p.Managers,
	}
}
