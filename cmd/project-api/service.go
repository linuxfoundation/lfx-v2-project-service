// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

// ProjectsAPI implements the projsvc.Service interface
type ProjectsAPI struct {
	service *service.ProjectsService
}

// NewProjectsAPI creates a new ProjectsAPI.
func NewProjectsAPI(service *service.ProjectsService) *ProjectsAPI {
	return &ProjectsAPI{
		service: service,
	}
}
