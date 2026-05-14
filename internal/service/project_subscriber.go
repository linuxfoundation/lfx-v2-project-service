// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service/email"
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
	slog.DebugContext(ctx, "project_subscriber: received project_settings.updated event",
		"project_uid", event.ProjectUID, "new_member_count", len(additions))
	if len(additions) == 0 {
		return nil
	}

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to load project", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	baseURL := strings.TrimRight(s.Config.LFXSelfServeBaseURL, "/") + "/project/overview"
	projectURL := baseURL
	if projectBase.Slug != "" {
		projectURL = baseURL + "?project=" + projectBase.Slug
	}

	inviterName := s.resolveActorDisplayName(ctx, event.Actor)

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

			subject, html, text, err := email.RenderProjectRoleNotification(email.ProjectRoleNotificationData{
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
			} else {
				slog.DebugContext(gctx, "project_subscriber: sent role notification email",
					"role", add.Role, "project_uid", event.ProjectUID, "to", add.User.Email)
			}
			return nil
		})
	}

	_ = g.Wait()
	return nil
}

// resolveActorDisplayName looks up the actor's display name from the auth service.
// Falls back to "A project administrator" if the lookup fails or returns no name.
func (s *ProjectsService) resolveActorDisplayName(ctx context.Context, actor events.Actor) string {
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Username != "" && s.UserReader != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, emailSendTimeout)
		defer cancel()
		if meta, err := s.UserReader.UserMetadataByPrincipal(lookupCtx, actor.Username); err == nil && meta != nil {
			if meta.Name != "" {
				return meta.Name
			}
			if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
				return full
			}
		}
	}
	return "A project administrator"
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
	// Index every identity key from old so a user matched by either
	// username or email is recognised regardless of which field was set.
	oldSet := make(map[string]struct{}, len(old)*2)
	for _, u := range old {
		for _, key := range memberKeys(u) {
			oldSet[key] = struct{}{}
		}
	}
	seenNew := make(map[string]struct{}, len(new)*2)
	var additions []roleAssignment
	for _, u := range new {
		keys := memberKeys(u)
		if len(keys) == 0 {
			continue
		}
		// Skip if this user was already seen under any of their identity keys,
		// covering cases where the same person appears with different identity
		// shapes (e.g. username+email in one entry, email-only in another).
		alreadySeen := false
		for _, key := range keys {
			if _, ok := seenNew[key]; ok {
				alreadySeen = true
				break
			}
		}
		if alreadySeen {
			continue
		}
		for _, key := range keys {
			seenNew[key] = struct{}{}
		}
		// The user is already present if ANY of their keys appear in oldSet.
		present := false
		for _, key := range keys {
			if _, ok := oldSet[key]; ok {
				present = true
				break
			}
		}
		if !present {
			additions = append(additions, roleAssignment{User: u, Role: role})
		}
	}
	return additions
}

// memberKeys returns all stable identity keys for a user.
// Username key comes first (preferred); Email key is appended when present.
// Returns an empty slice if neither field is set.
func memberKeys(u events.UserInfo) []string {
	var keys []string
	if u.Username != "" {
		keys = append(keys, "username:"+u.Username)
	}
	if u.Email != "" {
		keys = append(keys, "email:"+u.Email)
	}
	return keys
}
