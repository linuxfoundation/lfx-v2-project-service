// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
		slog.WarnContext(ctx, "project_subscriber: failed to unmarshal project settings updated event", constants.ErrKey, err)
		return nil
	}

	additions := diffNewMembers(event.OldSettings, event.NewSettings)
	if len(additions) == 0 {
		return nil
	}

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to load project", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	projectURL := fmt.Sprintf("%s/projects/%s", strings.TrimRight(s.Config.LFXSelfServeBaseURL, "/"), projectBase.Slug)

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
			if add.User.Email == "" {
				slog.WarnContext(gctx, "project_subscriber: skipping email — recipient has no email address",
					"role", add.Role, "username", add.User.Username, "project_uid", event.ProjectUID)
				return nil
			}

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
				slog.WarnContext(gctx, "project_subscriber: failed to render email template",
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
				slog.WarnContext(gctx, "project_subscriber: failed to send role notification email",
					constants.ErrKey, sendErr, "role", add.Role, "project_uid", event.ProjectUID)
			}
			return nil
		})
	}

	_ = g.Wait()
	return nil
}

// roleAssignment pairs a user with the role they were added to.
type roleAssignment struct {
	User events.UserInfo
	Role string
}

// diffNewMembers returns the users that appear in newSettings but not in oldSettings,
// across writers, auditors, and meeting_coordinators. Users are matched by Username
// when present, otherwise by Email. Users with neither Username nor Email are skipped.
func diffNewMembers(oldSettings, newSettings events.ProjectSettings) []roleAssignment {
	var additions []roleAssignment
	additions = append(additions, diffRole(oldSettings.Writers, newSettings.Writers, "Writer")...)
	additions = append(additions, diffRole(oldSettings.Auditors, newSettings.Auditors, "Auditor")...)
	additions = append(additions, diffRole(oldSettings.MeetingCoordinators, newSettings.MeetingCoordinators, "Meeting Coordinator")...)
	return additions
}

func diffRole(old, new []events.UserInfo, role string) []roleAssignment {
	oldSet := make(map[string]struct{}, len(old))
	for _, u := range old {
		if key := memberKey(u); key != "" {
			oldSet[key] = struct{}{}
		}
	}
	seenNew := make(map[string]struct{}, len(new))
	var additions []roleAssignment
	for _, u := range new {
		key := memberKey(u)
		if key == "" {
			continue
		}
		if _, alreadySeen := seenNew[key]; alreadySeen {
			continue
		}
		seenNew[key] = struct{}{}
		if _, exists := oldSet[key]; !exists {
			additions = append(additions, roleAssignment{User: u, Role: role})
		}
	}
	return additions
}

// memberKey returns a stable identity key for a user.
// Username takes priority; Email is the fallback. Returns "" if neither is set.
func memberKey(u events.UserInfo) string {
	if u.Username != "" {
		return "username:" + u.Username
	}
	if u.Email != "" {
		return "email:" + u.Email
	}
	return ""
}
