// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"time"

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
	FGAChecker         domain.AccessChecker // nil when OpenFGA is disabled (local dev)
	Config             ServiceConfig

	// now returns the current time; tests override it to pin the clock. A nil
	// value falls back to time.Now (see currentTime).
	now func() time.Time
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
func NewProjectsService(config ServiceConfig, deps ServiceDeps) *ProjectsService {
	return &ProjectsService{
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
		now:                time.Now,
	}
}

// currentTime returns the service clock, defaulting to time.Now when the
// service was built without NewProjectsService.
func (s *ProjectsService) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// ServiceReady checks if the service is ready for use.
func (s *ProjectsService) ServiceReady() bool {
	return s.ProjectRepository != nil && s.Publisher != nil && s.Sender != nil &&
		s.DocumentRepository != nil && s.LinkRepository != nil && s.FolderRepository != nil &&
		s.UserReader != nil && s.Resolver != nil && s.Dispatcher != nil
}

// publishIndexer sends msg to the NATS indexer subject in a background goroutine
// using a detached context, so the publish does not block the caller and is not
// cancelled when the request context ends.
func (s *ProjectsService) publishIndexer(ctx context.Context, subject string, msg any) {
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		if err := s.Publisher.SendIndexerMessage(bgCtx, subject, msg); err != nil {
			slog.WarnContext(bgCtx, "error sending indexer message", constants.ErrKey, err)
		}
	}()
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
	// ArchivedDissolutionDateRequiredFrom is a temporary waiver (LFXV2-1733) of the
	// rule that an Archived project must carry an entity dissolution date, so that
	// legacy v1 archived projects without one can be imported. It applies to
	// project creation only; updates always enforce the rule. While the current
	// UTC time is before this instant the rule is skipped on create; from this
	// instant on it is enforced, so the waiver ends by itself without a code change. The zero
	// value means no waiver (always enforced). Set via
	// ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM (YYYY-MM-DD, UTC).
	ArchivedDissolutionDateRequiredFrom time.Time
}
