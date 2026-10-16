// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

// ProjectsAPI implements the projsvc.Service interface
type ProjectsAPI struct {
	service *service.ProjectsService
	auth    domain.Authenticator
}

// NewProjectsAPI creates a new ProjectsAPI. The authenticator backs the JWT
// security handler; it is held here rather than reached through the service so
// the endpoint layer does not depend on the service's infrastructure fields.
func NewProjectsAPI(service *service.ProjectsService, auth domain.Authenticator) *ProjectsAPI {
	return &ProjectsAPI{
		service: service,
		auth:    auth,
	}
}
