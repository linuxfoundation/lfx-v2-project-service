// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// ProjectsService implements the projsvc.Service interface and domain.MessageHandler
type ProjectsService struct {
	ProjectRepository  domain.ProjectRepository
	DocumentRepository domain.DocumentRepository
	LinkRepository     domain.LinkRepository
	FolderRepository   domain.FolderRepository
	Publisher          domain.EventPublisher
	Sender             domain.OutboundRPC
	UserReader         domain.UserReader
	Resolver           *UserResolver
	Dispatcher         *NotificationDispatcher
	Auth               domain.Authenticator
	FGAChecker         domain.AccessChecker // nil when OpenFGA is disabled (local dev)
	Config             ServiceConfig
}

// ServiceDeps holds the infrastructure dependencies required by ProjectsService.
// All fields must be non-nil for the service to be ready (see ServiceReady).
// FGAChecker is optional: when nil, parent-change authorization checks are skipped
// (mirrors the Heimdall allow_all behaviour when openfga.enabled is false).
type ServiceDeps struct {
	ProjectRepository  domain.ProjectRepository
	DocumentRepository domain.DocumentRepository
	LinkRepository     domain.LinkRepository
	FolderRepository   domain.FolderRepository
	Publisher          domain.EventPublisher
	Sender             domain.OutboundRPC
	UserReader         domain.UserReader
	Resolver           *UserResolver
	Dispatcher         *NotificationDispatcher
	FGAChecker         domain.AccessChecker
}

// NewProjectsService creates a fully valid ProjectsService with all dependencies
// wired at construction time. Passing a zero ServiceDeps is allowed in tests that
// intentionally probe the not-ready state, but production callers must supply all fields.
func NewProjectsService(auth domain.Authenticator, config ServiceConfig, deps ServiceDeps) *ProjectsService {
	return &ProjectsService{
		Auth:               auth,
		Config:             config,
		ProjectRepository:  deps.ProjectRepository,
		DocumentRepository: deps.DocumentRepository,
		LinkRepository:     deps.LinkRepository,
		FolderRepository:   deps.FolderRepository,
		Publisher:          deps.Publisher,
		Sender:             deps.Sender,
		UserReader:         deps.UserReader,
		Resolver:           deps.Resolver,
		Dispatcher:         deps.Dispatcher,
		FGAChecker:         deps.FGAChecker,
	}
}

// ServiceReady checks if the service is ready for use.
func (s *ProjectsService) ServiceReady() bool {
	return s.ProjectRepository != nil && s.Publisher != nil && s.Sender != nil &&
		s.DocumentRepository != nil && s.LinkRepository != nil && s.FolderRepository != nil &&
		s.UserReader != nil && s.Resolver != nil && s.Dispatcher != nil
}

// publishIndexer sends msg to the NATS indexer subject. When sync is true the
// call blocks and the error (if any) is returned. When sync is false the publish
// runs in a background goroutine with a detached context and this method always
// returns nil immediately.
func (s *ProjectsService) publishIndexer(ctx context.Context, subject string, msg any, sync bool) error {
	if sync {
		if err := s.Publisher.SendIndexerMessage(ctx, subject, msg); err != nil {
			slog.WarnContext(ctx, "error sending indexer message", constants.ErrKey, err)
			return err
		}
		return nil
	}
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		if err := s.Publisher.SendIndexerMessage(bgCtx, subject, msg); err != nil {
			slog.WarnContext(bgCtx, "error sending indexer message", constants.ErrKey, err)
		}
	}()
	return nil
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
