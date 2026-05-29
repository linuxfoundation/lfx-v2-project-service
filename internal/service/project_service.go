// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
)

// ProjectsService implements the projsvc.Service interface and domain.MessageHandler
type ProjectsService struct {
	ProjectRepository  domain.ProjectRepository
	DocumentRepository domain.DocumentRepository
	LinkRepository     domain.LinkRepository
	FolderRepository   domain.FolderRepository
	MessageBuilder     domain.MessageBuilder
	UserReader         domain.UserReader
	Auth               domain.Authenticator
	Config             ServiceConfig
}

// NewProjectsService creates a new ProjectsService.
func NewProjectsService(auth domain.Authenticator, config ServiceConfig) *ProjectsService {
	return &ProjectsService{
		Auth:   auth,
		Config: config,
	}
}

// ServiceReady checks if the service is ready for use.
func (s *ProjectsService) ServiceReady() bool {
	return s.ProjectRepository != nil && s.MessageBuilder != nil &&
		s.DocumentRepository != nil && s.LinkRepository != nil && s.FolderRepository != nil &&
		s.UserReader != nil
}

// ServiceConfig is the configuration for the ProjectsService.
type ServiceConfig struct {
	// SkipEtagValidation is a flag to skip the Etag validation - only meant for local development.
	SkipEtagValidation bool
	// LFXSelfServeBaseURL is the base URL for LFX Self-Serve, used to build project URLs in notification emails.
	LFXSelfServeBaseURL string
	// EmailsEnabled gates outbound role-notification emails to LFID users via the email service.
	// Disabled by default; set EMAILS_ENABLED=true to enable.
	EmailsEnabled bool
	// InvitesEnabled gates outbound invite requests for non-LFID users via the invite service.
	// Disabled by default; set INVITES_ENABLED=true to enable.
	InvitesEnabled bool
}
