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
)

// HandleMessage implements domain.MessageHandler interface
func (s *ProjectsService) HandleMessage(msg domain.Message) {
	subject := msg.Subject()
	ctx := log.AppendCtx(context.Background(), slog.String("subject", subject))
	slog.DebugContext(ctx, "handling NATS message")

	var response []byte
	var err error
	switch subject {
	case constants.ProjectGetNameSubject:
		response, err = s.HandleProjectGetName(msg)
		if err != nil {
			slog.ErrorContext(ctx, "error handling project get name", constants.ErrKey, err)
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
	case constants.ProjectSlugToUIDSubject:
		response, err = s.HandleProjectSlugToUID(msg)
		if err != nil {
			slog.ErrorContext(ctx, "error handling project slug to UID", constants.ErrKey, err)
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
	default:
		slog.WarnContext(ctx, "unknown subject")
		err = msg.Respond(nil)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", constants.ErrKey, err)
			return
		}
	}

	slog.DebugContext(ctx, "responded to NATS message", "response", response)
}

// HandleProjectGetName is the message handler for the project-get-name subject.
func (s *ProjectsService) HandleProjectGetName(msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(context.Background(), "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectID := string(msg.Data())

	ctx := log.AppendCtx(context.Background(), slog.String("project_id", projectID))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectGetNameSubject))

	// Validate that the project ID is a valid UUID.
	_, err := uuid.Parse(projectID)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing project ID", constants.ErrKey, err)
		return nil, err
	}

	project, err := s.ProjectRepository.GetProjectBase(ctx, projectID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project from NATS KV", constants.ErrKey, err)
		return nil, err
	}

	return []byte(project.Name), nil
}

// HandleProjectSlugToUID is the message handler for the project-slug-to-uid subject.
func (s *ProjectsService) HandleProjectSlugToUID(msg domain.Message) ([]byte, error) {
	if !s.ServiceReady() {
		slog.ErrorContext(context.Background(), "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	projectSlug := string(msg.Data())

	ctx := log.AppendCtx(context.Background(), slog.String("project_slug", projectSlug))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectSlugToUIDSubject))

	project, err := s.ProjectRepository.GetProjectUIDFromSlug(ctx, projectSlug)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project UID from repository", constants.ErrKey, err)
		return nil, err
	}

	return []byte(project), nil
}
