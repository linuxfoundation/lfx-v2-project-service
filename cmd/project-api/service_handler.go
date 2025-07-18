// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
)

// INatsMsg is an interface for [nats.Msg] that allows for mocking.
type INatsMsg interface {
	Respond(data []byte) error
	Data() []byte
	Subject() string
}

// NatsMsg is a wrapper around [nats.Msg] that implements [INatsMsg].
type NatsMsg struct {
	*nats.Msg
}

// Respond implements [INatsMsg.Respond].
func (m *NatsMsg) Respond(data []byte) error {
	return m.Msg.Respond(data)
}

// Data implements [INatsMsg.Data].
func (m *NatsMsg) Data() []byte {
	return m.Msg.Data
}

// Subject implements [INatsMsg.Subject].
func (m *NatsMsg) Subject() string {
	return m.Msg.Subject
}

// HandleNatsMessage is the entrypoint NATS handler for all subjects handled by the service.
func (s *ProjectsService) HandleNatsMessage(msg INatsMsg) {
	subject := msg.Subject()
	ctx := log.AppendCtx(context.Background(), slog.String("subject", subject))
	slog.DebugContext(ctx, "handling NATS message")

	var response []byte
	var err error
	switch subject {
	case fmt.Sprintf("%s%s", s.lfxEnvironment, constants.ProjectGetNameSubject):
		response, err = s.HandleProjectGetName(msg)
		if err != nil {
			slog.ErrorContext(ctx, "error handling project get name", errKey, err)
			err = msg.Respond(nil)
			if err != nil {
				slog.ErrorContext(ctx, "error responding to NATS message", errKey, err)
			}
			return
		}
		err = msg.Respond(response)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", errKey, err)
			return
		}
	case fmt.Sprintf("%s%s", s.lfxEnvironment, constants.ProjectSlugToUIDSubject):
		response, err = s.HandleProjectSlugToUID(msg)
		if err != nil {
			slog.ErrorContext(ctx, "error handling project slug to UID", errKey, err)
			err = msg.Respond(nil)
			if err != nil {
				slog.ErrorContext(ctx, "error responding to NATS message", errKey, err)
			}
			return
		}
		err = msg.Respond(response)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", errKey, err)
			return
		}
	default:
		slog.WarnContext(ctx, "unknown subject")
		err = msg.Respond(nil)
		if err != nil {
			slog.ErrorContext(ctx, "error responding to NATS message", errKey, err)
			return
		}
	}

	slog.DebugContext(ctx, "responded to NATS message", "response", response)
}

// HandleProjectGetName is the NATS handler for the project-get-name subject.
func (s *ProjectsService) HandleProjectGetName(msg INatsMsg) ([]byte, error) {
	projectID := string(msg.Data())

	ctx := log.AppendCtx(context.Background(), slog.String("project_id", projectID))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectGetNameSubject))

	// Validate that the project ID is a valid UUID.
	_, err := uuid.Parse(projectID)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing project ID", errKey, err)
		return nil, err
	}

	if s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	project, err := s.projectsKV.Get(ctx, projectID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project from NATS KV", errKey, err)
		return nil, err
	}

	return project.Value(), nil
}

// HandleProjectSlugToUID is the NATS handler for the project-slug-to-uid subject.
func (s *ProjectsService) HandleProjectSlugToUID(msg INatsMsg) ([]byte, error) {
	projectSlug := string(msg.Data())

	ctx := log.AppendCtx(context.Background(), slog.String("project_slug", projectSlug))
	ctx = log.AppendCtx(ctx, slog.String("subject", constants.ProjectSlugToUIDSubject))

	if s.projectsKV == nil {
		slog.ErrorContext(ctx, "NATS KV store not initialized")
		return nil, fmt.Errorf("NATS KV store not initialized")
	}

	project, err := s.projectsKV.Get(ctx, fmt.Sprintf("slug/%s", projectSlug))
	if err != nil {
		slog.ErrorContext(ctx, "error getting project from NATS KV", errKey, err)
		return nil, err
	}

	return project.Value(), nil
}
