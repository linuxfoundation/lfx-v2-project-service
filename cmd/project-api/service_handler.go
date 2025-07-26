// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log/slog"

	internalnats "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// HandleNatsMessage is the entrypoint NATS handler for all subjects handled by the service.
func (s *ProjectsAPI) HandleNatsMessage(msg internalnats.INatsMsg) {
	subject := msg.Subject()
	ctx := log.AppendCtx(context.Background(), slog.String("subject", subject))
	slog.DebugContext(ctx, "handling NATS message")

	var response []byte
	var err error
	switch subject {
	case constants.ProjectGetNameSubject:
		response, err = s.service.HandleProjectGetName(msg)
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
	case constants.ProjectSlugToUIDSubject:
		response, err = s.service.HandleProjectSlugToUID(msg)
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
