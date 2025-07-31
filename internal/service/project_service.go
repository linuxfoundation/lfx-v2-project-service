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
	Config            ServiceConfig
}

// NewProjectsService creates a new ProjectsService.
func NewProjectsService(auth auth.IJWTAuth, config ServiceConfig) *ProjectsService {
	return &ProjectsService{
		Auth:   auth,
		Config: config,
	}
}

// ServiceReady checks if the service is ready for use.
func (s *ProjectsService) ServiceReady() bool {
	return s.ProjectRepository != nil && s.MessageBuilder != nil
}

// ServiceConfig is the configuration for the ProjectsService.
type ServiceConfig struct {
	// SkipEtagValidation is a flag to skip the Etag validation - only meant for local development.
	SkipEtagValidation bool
}
