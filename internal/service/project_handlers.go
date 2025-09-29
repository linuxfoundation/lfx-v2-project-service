// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
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
		constants.ProjectGetNameSubject:   s.HandleProjectGetName,
		constants.ProjectGetSlugSubject:   s.HandleProjectGetSlug,
		constants.ProjectGetLogoSubject:   s.HandleProjectGetLogo,
		constants.ProjectSlugToUIDSubject: s.HandleProjectSlugToUID,
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
		slog.ErrorContext(ctx, "error handling message",
			constants.ErrKey, err,
			"subject", subject,
		)
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
		slog.ErrorContext(ctx, "error parsing project ID", constants.ErrKey, err)
		return nil, err
	}

	project, err := s.ProjectRepository.GetProjectBase(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project from NATS KV", constants.ErrKey, err)
		return nil, err
	}

	value, ok := structs.FieldByTag(project, "json", getAttribute)
	if !ok {
		slog.ErrorContext(ctx, "error getting project attribute", constants.ErrKey, fmt.Errorf("attribute %s not found", getAttribute))
		return nil, fmt.Errorf("attribute %s not found", getAttribute)
	}

	strValue, ok := value.(string)
	if !ok {
		slog.ErrorContext(ctx, "project attribute is not a string", constants.ErrKey, fmt.Errorf("attribute %s is not a string", getAttribute))
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
		slog.ErrorContext(ctx, "error getting project UID from repository", constants.ErrKey, err)
		return nil, err
	}

	return []byte(project), nil
}
