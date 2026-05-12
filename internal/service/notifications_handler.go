// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"golang.org/x/sync/errgroup"
)

const emailSendTimeout = 5 * time.Second

// HandleProjectSettingsUpdated handles project_settings.updated events and sends
// notification emails to any users newly added as writers, auditors, or meeting coordinators.
// Errors from individual sends are logged but never returned — the handler is best-effort.
func (s *ProjectsService) HandleProjectSettingsUpdated(ctx context.Context, msg domain.Message) error {
	var event events.ProjectSettingsUpdatedMessage
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "notifications_handler: failed to unmarshal project settings updated event", constants.ErrKey, err)
		return nil
	}

	additions := diffNewMembers(event.OldSettings, event.NewSettings)
	if len(additions) == 0 {
		return nil
	}

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "notifications_handler: failed to load project", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	projectURL := fmt.Sprintf("%s/projects/%s", s.Config.LFXSelfServeBaseURL, projectBase.Slug)

	inviterName := event.Actor.Name
	if inviterName == "" {
		inviterName = event.Actor.Username
	}
	if inviterName == "" {
		inviterName = "A project administrator"
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, add := range additions {
		add := add
		g.Go(func() error {
			recipientName := add.User.Name
			if recipientName == "" {
				recipientName = add.User.Username
			}
			if recipientName == "" {
				recipientName = add.User.Email
			}

			subject, html, text, err := renderProjectRoleNotification(projectRoleNotificationData{
				RecipientName: recipientName,
				ProjectName:   projectBase.Name,
				Role:          add.Role,
				ProjectURL:    projectURL,
				InviterName:   inviterName,
			})
			if err != nil {
				slog.WarnContext(gctx, "notifications_handler: failed to render email template",
					constants.ErrKey, err, "role", add.Role, "project_uid", event.ProjectUID)
				return nil
			}

			sendCtx, cancel := context.WithTimeout(gctx, emailSendTimeout)
			defer cancel()
			sendErr := s.MessageBuilder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
				To:      add.User.Email,
				Subject: subject,
				HTML:    html,
				Text:    text,
			})
			if sendErr != nil {
				slog.WarnContext(gctx, "notifications_handler: failed to send role notification email",
					constants.ErrKey, sendErr, "role", add.Role, "project_uid", event.ProjectUID)
			}
			return nil
		})
	}

	_ = g.Wait()
	return nil
}
