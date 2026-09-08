// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	structs "github.com/linuxfoundation/lfx-v2-project-service/pkg/struct"
)

// HandleMessage implements domain.MessageHandler interface
func (s *ProjectsService) HandleMessage(ctx context.Context, msg domain.Message) {
	subject := msg.Subject()
	ctx = log.AppendCtx(ctx, slog.String("subject", subject))
	slog.DebugContext(ctx, "handling NATS message")

	var response []byte
	var err error

	handlers := map[string]func(ctx context.Context, msg domain.Message) ([]byte, error){
		constants.ProjectGetNameSubject:      s.HandleProjectGetName,
		constants.ProjectGetSlugSubject:      s.HandleProjectGetSlug,
		constants.ProjectGetLogoSubject:      s.HandleProjectGetLogo,
		constants.ProjectSlugToUIDSubject:    s.HandleProjectSlugToUID,
		constants.ProjectGetParentUIDSubject: s.HandleProjectGetParentUID,
		constants.ProjectGetWritersSubject:   s.HandleProjectGetWriters,
		constants.ProjectGetSettingsSubject:  s.HandleProjectGetSettings,
		constants.ProjectListProjectsSubject: s.HandleProjectListProjects,
	}

	handler, ok := handlers[subject]
	if !ok {
		slog.WarnContext(ctx, "unknown subject")
		err = msg.Respond(nil)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", constants.ErrKey, err)
			return
		}
		return
	}

	response, err = handler(ctx, msg)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			slog.WarnContext(ctx, "project not found while handling message",
				constants.ErrKey, err,
			)
		} else {
			slog.ErrorContext(ctx, "error handling message",
				constants.ErrKey, err,
			)
		}
		err = msg.Respond(nil)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", constants.ErrKey, err)
		}
		return
	}
	err = msg.Respond(response)
	if err != nil {
		slog.ErrorContext(ctx, "error responding to NATS message", constants.ErrKey, err)
		return
	}

	slog.DebugContext(ctx, "responded to NATS message", "response", response)
}

func (s *ProjectsService) handleProjectGetAttribute(ctx context.Context, msg domain.Message, subject, getAttribute string) ([]byte, error) {

	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectUID := string(msg.Data())

	ctx = log.AppendCtx(ctx, slog.String("project_id", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("subject", subject))

	// Validate that the project ID is a valid UUID.
	_, err := uuid.Parse(projectUID)
	if err != nil {
		return nil, err
	}

	project, err := s.ProjectRepository.GetProjectBase(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	value, ok := structs.FieldByTag(project, "json", getAttribute)
	if !ok {
		return nil, fmt.Errorf("attribute %s not found", getAttribute)
	}

	strValue, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("attribute %s is not a string", getAttribute)
	}

	return []byte(strValue), nil
}

// HandleProjectGetName is the message handler for the project-get-name subject.
func (s *ProjectsService) HandleProjectGetName(ctx context.Context, msg domain.Message) ([]byte, error) {
	return s.handleProjectGetAttribute(ctx, msg, constants.ProjectGetNameSubject, "name")
}

// HandleProjectGetSlug is the message handler for the project-get-slug subject.
func (s *ProjectsService) HandleProjectGetSlug(ctx context.Context, msg domain.Message) ([]byte, error) {
	return s.handleProjectGetAttribute(ctx, msg, constants.ProjectGetSlugSubject, "slug")
}

// HandleProjectGetLogo is the message handler for the project-get-logo subject.
func (s *ProjectsService) HandleProjectGetLogo(ctx context.Context, msg domain.Message) ([]byte, error) {
	return s.handleProjectGetAttribute(ctx, msg, constants.ProjectGetLogoSubject, "logo_url")
}

// HandleProjectSlugToUID is the message handler for the project-slug-to-uid subject.
func (s *ProjectsService) HandleProjectSlugToUID(ctx context.Context, msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectSlug := string(msg.Data())

	ctx = log.AppendCtx(ctx, slog.String("project_slug", projectSlug))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectSlugToUIDSubject))

	project, err := s.ProjectRepository.GetProjectUIDFromSlug(ctx, projectSlug)
	if err != nil {
		return nil, err
	}

	return []byte(project), nil
}

// HandleProjectGetParentUID is the message handler for the project-get-parent-uid subject.
func (s *ProjectsService) HandleProjectGetParentUID(ctx context.Context, msg domain.Message) ([]byte, error) {
	return s.handleProjectGetAttribute(ctx, msg, constants.ProjectGetParentUIDSubject, "parent_uid")
}

// HandleProjectGetWriters is the message handler for the project-get-writers subject.
// Request: plain-text project UID. Reply: JSON-encoded []models.UserInfo writers list.
// Returns an empty JSON array when the project has no writers configured.
func (s *ProjectsService) HandleProjectGetWriters(ctx context.Context, msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectUID := string(msg.Data())

	ctx = log.AppendCtx(ctx, slog.String("project_id", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectGetWritersSubject))

	_, err := uuid.Parse(projectUID)
	if err != nil {
		return nil, err
	}

	settings, err := s.ProjectRepository.GetProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	writers := settings.Writers
	if writers == nil {
		writers = []models.UserInfo{}
	}

	out, err := json.Marshal(writers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal writers: %w", err)
	}

	return out, nil
}

// HandleProjectGetSettings is the message handler for the project-get-settings subject.
// Request: plain-text project UID. Reply: JSON-encoded events.ProjectSettingsSummary.
//
// It returns the writers, the auditors and the announcement date together, from the one
// settings record that holds all three. Callers that need the full grant roster cannot
// assemble it from get_writers, which returns half of it — and a caller that treats half
// a roster as the whole one refuses everyone in the missing half.
func (s *ProjectsService) HandleProjectGetSettings(ctx context.Context, msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectUID := string(msg.Data())

	ctx = log.AppendCtx(ctx, slog.String("project_uid", projectUID))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectGetSettingsSubject))

	_, err := uuid.Parse(projectUID)
	if err != nil {
		return nil, err
	}

	settings, err := s.ProjectRepository.GetProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	out, err := json.Marshal(DomainSettingsToSummary(settings))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal project settings summary: %w", err)
	}

	return out, nil
}

// HandleProjectListProjects is the message handler for the project-list-projects subject.
// Request: JSON-encoded events.ProjectListRequest. Reply: JSON-encoded []events.ProjectRef.
//
// The stage filter and the UID filter are a union, so one request can ask both "which
// projects are at these stages" and "what stage are these particular projects at now" —
// the second being the question a caller has to ask about projects it holds state for
// that may since have left the stages it was watching.
//
// The two filters are also served differently, because their costs differ: the stage
// filter has to scan the store, while a named UID is a direct read. A request carrying
// only UIDs therefore does no scan at all, and a request carrying both reads nothing
// twice — the scan already decoded every project, so a named UID is served from it.
//
// A UID naming no project is skipped rather than failing the request. Projects are
// deleted, and a caller holding a stale UID should get an answer about the rest instead
// of an error about the one. A malformed UID is a different matter and does fail: that
// is the caller sending something it never could have read from here.
func (s *ProjectsService) HandleProjectListProjects(ctx context.Context, msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectListProjectsSubject))

	var request events.ProjectListRequest
	if err := json.Unmarshal(msg.Data(), &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal project list request: %w", err)
	}

	if len(request.Stages) == 0 && len(request.UIDs) == 0 {
		return nil, fmt.Errorf("at least one of stages or uids is required")
	}

	// Every UID is validated before any read, so a request that is going to be refused
	// for a malformed UID is refused before it pays for the store scan below.
	for _, projectUID := range request.UIDs {
		if _, err := uuid.Parse(projectUID); err != nil {
			return nil, err
		}
	}

	refs := []events.ProjectRef{}
	// Tracks what the reply already holds, so a project matched by stage and also named
	// in the UID filter is returned once. The overlap is expected rather than a caller
	// error: a caller listing the stages it watches has no way to know which of the UIDs
	// it holds are still at one of them, which is the question it is asking.
	included := make(map[string]bool)
	// Every project the scan decoded, whatever its stage, so the UID filter below can be
	// answered from it. The scan reads the whole store either way, and the projects it
	// discards for being at an unwanted stage are exactly the ones a caller asks about by
	// UID — re-reading those would pay twice for a record already in hand.
	scanned := make(map[string]*models.ProjectBase)

	if len(request.Stages) > 0 {
		wanted := make(map[string]bool, len(request.Stages))
		for _, stage := range request.Stages {
			wanted[stage] = true
		}

		projects, err := s.ProjectRepository.ListAllProjectsBase(ctx)
		if err != nil {
			return nil, err
		}

		for _, project := range projects {
			if project == nil {
				continue
			}
			scanned[project.UID] = project

			if !wanted[project.Stage] {
				continue
			}
			refs = append(refs, DomainProjectToRef(project))
			included[project.UID] = true
		}
	}

	for _, projectUID := range request.UIDs {
		if included[projectUID] {
			continue
		}

		project, ok := scanned[projectUID]
		if !ok {
			var err error
			project, err = s.ProjectRepository.GetProjectBase(ctx, projectUID)
			if err != nil {
				if errors.Is(err, domain.ErrProjectNotFound) {
					slog.DebugContext(ctx, "skipping requested project that no longer exists",
						"project_uid", projectUID)
					continue
				}
				return nil, err
			}
		}

		refs = append(refs, DomainProjectToRef(project))
		included[projectUID] = true
	}

	// The store iterates keys in no defined order, so without this the same request
	// answers in a different order each time. Sorting costs nothing at this size and
	// spares every caller from having to not depend on it.
	slices.SortFunc(refs, func(a, b events.ProjectRef) int { return strings.Compare(a.UID, b.UID) })

	out, err := json.Marshal(refs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal project list: %w", err)
	}

	return out, nil
}
