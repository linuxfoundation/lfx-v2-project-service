// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
)

// ProjectsService implements the projsvc.Service interface and domain.MessageHandler
type ProjectsService struct {
	ProjectRepository domain.ProjectRepository
	MessageBuilder    domain.MessageBuilder
	Auth              auth.IJWTAuth
}

// NewProjectsService creates a new ProjectsService.
func NewProjectsService(repo domain.ProjectRepository, auth auth.IJWTAuth) *ProjectsService {
	return &ProjectsService{
		ProjectRepository: repo,
		Auth:              auth,
	}
}

// ServiceReady checks if the service is ready for use.
func (s *ProjectsService) ServiceReady() bool {
	return s.ProjectRepository != nil && s.MessageBuilder != nil
}
