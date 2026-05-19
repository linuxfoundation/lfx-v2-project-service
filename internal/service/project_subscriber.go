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
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service/email"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"golang.org/x/sync/errgroup"
)

// notificationTimeout caps blocking outbound calls (email-service request/reply and
// auth-service actor name lookup) to avoid blocking the event handler.
// Fire-and-forget NATS publishes (invite path) do not use this timeout.
const notificationTimeout = 5 * time.Second

const (
	roleWriter             = "Writer"
	roleAuditor            = "Auditor"
	roleMeetingCoordinator = "Meeting Coordinator"
)

// HandleProjectSettingsUpdated handles project_settings.updated events and sends
// notification emails to any users newly added as writers, auditors, or meeting coordinators.
// Users with an LFID (Username present) receive a direct notification email via the email
// service. Users without an LFID (email-only) receive an invite via the invite service so
// they can create an LFID and gain access.
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
		g.Go(func() error {
			if add.User.Email == "" {
				slog.WarnContext(gctx, "project_subscriber: skipping notification — recipient has no email address",
					"role", add.Role, "project_uid", event.ProjectUID)
				return nil
			}

			recipientName := add.User.Name
			if recipientName == "" {
				recipientName = add.User.Username
			}
			if recipientName == "" {
				recipientName = add.User.Email
			}

			if add.User.Username == "" {
				// Username == "" means no LFID yet; route through the invite service
				// so the user must create an account before gaining project access.
				return s.sendInvite(gctx, event.ProjectUID, projectBase.Name, add.Role, add.User.Email, recipientName, inviterName, projectURL)
			}

			// LFID present — send a direct notification email.
			return s.sendRoleNotificationEmail(gctx, event.ProjectUID, projectBase.Name, add.Role, add.User.Email, recipientName, inviterName, projectURL)
		})
	}

	_ = g.Wait()
	return nil
}

// sendInvite sends a send-invite request to the invite service for a user
// who does not yet have an LFID. The invite service renders and delivers the email.
// On success, the returned invite UID is written back to the project settings.
func (s *ProjectsService) sendInvite(ctx context.Context, projectUID, projectName, role, recipientEmail, recipientName, inviterName, deepLinkURL string) error {
	inviteRole := mapRoleToInviteRole(role)
	if inviteRole == "" {
		slog.WarnContext(ctx, "project_subscriber: skipping invite — unrecognised role",
			"role", role, "project_uid", projectUID)
		return nil
	}

	slog.InfoContext(ctx, "project_subscriber: sending invite request to invite service",
		"role", role, "invite_role", inviteRole, "project_uid", projectUID, "recipient_email", recipientEmail)

	sendCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
	defer cancel()

	result, err := s.MessageBuilder.SendInviteRequest(sendCtx, inviteapi.SendInviteRequest{
		RecipientEmail: recipientEmail,
		RecipientName:  recipientName,
		InviterName:    inviterName,
		ResourceUID:    projectUID,
		ResourceName:   projectName,
		ResourceType:   "project",
		Role:           inviteRole,
		ReturnURL:      deepLinkURL,
		ExpirationDays: 30,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to send invite request",
			constants.ErrKey, err, "role", role, "project_uid", projectUID)
		return nil
	}

	if result.InviteUID == "" {
		slog.WarnContext(ctx, "project_subscriber: invite service responded without an invite UID — skipping write-back",
			"role", role, "project_uid", projectUID, "recipient_email", recipientEmail)
		return nil
	}

	slog.InfoContext(ctx, "project_subscriber: invite service responded with invite UID — storing on member record",
		"role", role, "project_uid", projectUID, "invite_uid", result.InviteUID, "expires_at", result.ExpiresAt)

	if storeErr := s.storeInviteInfo(ctx, projectUID, role, recipientEmail, result.InviteUID, result.RecipientEmail, result.ExpiresAt); storeErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to store invite info on user",
			constants.ErrKey, storeErr, "role", role, "project_uid", projectUID, "invite_uid", result.InviteUID)
	}
	return nil
}

// storeInviteInfo reads project settings, locates the user by email in the given role's
// slice, stamps their InviteUID, InviteEmail, and InviteExpiresAt, and writes the settings back using optimistic concurrency.
func (s *ProjectsService) storeInviteInfo(ctx context.Context, projectUID, role, recipientEmail, inviteUID, inviteEmail string, expiresAt time.Time) error {
	slog.DebugContext(ctx, "project_subscriber: reading project settings to store invite info",
		"project_uid", projectUID, "role", role, "recipient_email", recipientEmail)

	settings, revision, err := s.ProjectRepository.GetProjectSettingsWithRevision(ctx, projectUID)
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to read project settings for invite info write-back",
			constants.ErrKey, err, "project_uid", projectUID)
		return err
	}

	var slice []models.UserInfo
	switch role {
	case roleWriter:
		slice = settings.Writers
	case roleAuditor:
		slice = settings.Auditors
	case roleMeetingCoordinator:
		slice = settings.MeetingCoordinators
	default:
		return nil
	}

	updated := false
	for i := range slice {
		if slice[i].Email == recipientEmail {
			inv := &models.InviteInfo{UID: inviteUID, Email: inviteEmail}
			if !expiresAt.IsZero() {
				inv.ExpiresAt = &expiresAt
			}
			slice[i].Invite = inv
			updated = true
			break
		}
	}
	if !updated {
		slog.WarnContext(ctx, "project_subscriber: user not found in role slice — invite info not stored (user may have been removed)",
			"project_uid", projectUID, "role", role, "recipient_email", recipientEmail, "invite_uid", inviteUID)
		return nil
	}

	switch role {
	case roleWriter:
		settings.Writers = slice
	case roleAuditor:
		settings.Auditors = slice
	case roleMeetingCoordinator:
		settings.MeetingCoordinators = slice
	}

	if err := s.ProjectRepository.UpdateProjectSettings(ctx, settings, revision); err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to write invite info back to project settings",
			constants.ErrKey, err, "project_uid", projectUID, "role", role, "invite_uid", inviteUID)
		return err
	}

	slog.InfoContext(ctx, "project_subscriber: stored invite info on member record",
		"project_uid", projectUID, "role", role, "recipient_email", recipientEmail, "invite_uid", inviteUID, "expires_at", expiresAt)
	return nil
}

// sendRoleNotificationEmail sends a direct "you were added" notification email via
// the email service for a user who already has an LFID.
func (s *ProjectsService) sendRoleNotificationEmail(ctx context.Context, projectUID, projectName, role, recipientEmail, recipientName, inviterName, projectURL string) error {
	subject, html, text, err := email.RenderProjectRoleNotification(email.ProjectRoleNotificationData{
		RecipientName: recipientName,
		ProjectName:   projectName,
		Role:          role,
		ProjectURL:    projectURL,
		InviterName:   inviterName,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to render email template",
			constants.ErrKey, err, "role", role, "project_uid", projectUID)
		return nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
	defer cancel()

	sendErr := s.MessageBuilder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
		To:      recipientEmail,
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
	if sendErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to send role notification email",
			constants.ErrKey, sendErr, "role", role, "project_uid", projectUID)
	} else {
		slog.DebugContext(ctx, "project_subscriber: sent role notification email",
			"role", role, "project_uid", projectUID)
	}
	return nil
}

// resolveActorDisplayName looks up the actor's display name from the auth service.
// Falls back to "A project administrator" if the lookup fails or returns no name.
func (s *ProjectsService) resolveActorDisplayName(ctx context.Context, actor events.Actor) string {
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Username != "" && s.UserReader != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
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

// mapRoleToInviteRole converts a project-service role string to the invite service's
// role vocabulary. Returns an empty string for unrecognised roles (caller skips invite).
//
// Mapping:
//   - Writer           → Manage
//   - Auditor          → View
//   - Meeting Coordinator → Manage (coordinators have write-level project access)
func mapRoleToInviteRole(role string) string {
	switch role {
	case roleWriter, roleMeetingCoordinator:
		return string(inviteapi.InviteRoleManage)
	case roleAuditor:
		return string(inviteapi.InviteRoleView)
	default:
		return ""
	}
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
	additions = append(additions, diffRole(oldSettings.Writers, newSettings.Writers, roleWriter)...)
	additions = append(additions, diffRole(oldSettings.Auditors, newSettings.Auditors, roleAuditor)...)
	additions = append(additions, diffRole(oldSettings.MeetingCoordinators, newSettings.MeetingCoordinators, roleMeetingCoordinator)...)
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
