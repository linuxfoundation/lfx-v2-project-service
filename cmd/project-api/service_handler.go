// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// INatsMsg is an interface for [nats.Msg] that allows for mocking.
type INatsMsg interface {
	Respond(data []byte) error
	Data() []byte
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

// HandleProjectGetName is the NATS handler for the project-get-name subject.
func (s *ProjectsService) HandleProjectGetName(msg INatsMsg) {
	projectID := string(msg.Data())

	logger := s.logger.With("project_id", projectID).With("handler", "project-get-name")
	logger.DebugContext(context.Background(), "handling NATS message")

	// Validate that the project ID is a valid UUID.
	_, err := uuid.Parse(projectID)
	if err != nil {
		logger.With(errKey, err).Error("error parsing project ID")
		msg.Respond(nil)
		return
	}

	if s.projectsKV == nil {
		logger.Error("NATS KV store not initialized")
		msg.Respond(nil)
		return
	}

	ctx := context.Background()
	project, err := s.projectsKV.Get(ctx, projectID)
	if err != nil {
		logger.With(errKey, err).Error("error getting project from NATS KV")
		msg.Respond(nil)
		return
	}

	logger.With("project", project).DebugContext(ctx, "responding to NATS message")
	err = msg.Respond(project.Value())
	if err != nil {
		logger.With(errKey, err).Error("error responding to NATS message")
		return
	}
}

// HandleProjectSlugToUID is the NATS handler for the project-slug-to-uid subject.
func (s *ProjectsService) HandleProjectSlugToUID(msg INatsMsg) {
	projectSlug := string(msg.Data())

	logger := s.logger.With("project_slug", projectSlug).With("handler", "project-slug-to-uid")

	if s.projectsKV == nil {
		logger.Error("NATS KV store not initialized")
		msg.Respond(nil)
		return
	}

	ctx := context.Background()
	project, err := s.projectsKV.Get(ctx, fmt.Sprintf("slug/%s", projectSlug))
	if err != nil {
		logger.With(errKey, err).Error("error getting project from NATS KV")
		msg.Respond(nil)
		return
	}

	err = msg.Respond(project.Value())
	if err != nil {
		logger.With(errKey, err).Error("error responding to NATS message")
		return
	}
}
