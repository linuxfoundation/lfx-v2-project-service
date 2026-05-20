// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service/email"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"golang.org/x/sync/errgroup"
)

// notificationTimeout caps blocking outbound calls to avoid stalling the event handler:
// email-service request/reply, invite-service request/reply (SendInviteRequest), and
// auth-service actor name lookup all run under this deadline.
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

			// Skip notification if this user was previously present as an email-only (invited)
			// entry — they're being promoted from non-LFID to LFID via invite acceptance, not
			// freshly added. They already received the invite email.
			if add.User.Username != "" && wasInvitedInOldSettings(add.User.Email, event.OldSettings) {
				slog.DebugContext(gctx, "project_subscriber: skipping notification — user promoted from invite to LFID",
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
// slice, stamps their InviteUID, InviteEmail, and InviteExpiresAt, and writes the settings
// back using optimistic concurrency. It retries up to 3 times on ErrRevisionMismatch,
// which can occur when multiple non-LFID users are added in the same event and concurrent
// write-backs race on the same KV revision.
func (s *ProjectsService) storeInviteInfo(ctx context.Context, projectUID, role, recipientEmail, inviteUID, inviteEmail string, expiresAt time.Time) error {
	const maxRetries = 3
	for attempt := range maxRetries {
		storeCtx, storeCancel := context.WithTimeout(ctx, notificationTimeout)
		err := s.tryStoreInviteInfo(storeCtx, projectUID, role, recipientEmail, inviteUID, inviteEmail, expiresAt)
		storeCancel()
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrRevisionMismatch) || attempt == maxRetries-1 {
			return err
		}
		slog.DebugContext(ctx, "project_subscriber: revision mismatch storing invite info — retrying",
			"attempt", attempt+1, "project_uid", projectUID, "role", role, "invite_uid", inviteUID)
	}
	return nil
}

func (s *ProjectsService) tryStoreInviteInfo(ctx context.Context, projectUID, role, recipientEmail, inviteUID, inviteEmail string, expiresAt time.Time) error {
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

	normalizedRecipient := strings.ToLower(strings.TrimSpace(recipientEmail))
	updated := false
	for i := range slice {
		if strings.ToLower(strings.TrimSpace(slice[i].Email)) == normalizedRecipient {
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

	// Write the invite UID → project UID mapping so HandleInviteAccepted can route the
	// acceptance event without scanning all project settings.
	if mappingErr := s.ProjectRepository.CreateInviteMapping(ctx, inviteUID, projectUID); mappingErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to create invite mapping — acceptance routing will not work",
			constants.ErrKey, mappingErr, "project_uid", projectUID, "invite_uid", inviteUID)
	}

	indexMsg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionUpdated,
		Data:           *settings,
		IndexingConfig: settings.IndexingConfig(projectUID),
	}
	if indexErr := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, indexMsg, false); indexErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to reindex project settings after storing invite info",
			constants.ErrKey, indexErr, "project_uid", projectUID, "role", role, "invite_uid", inviteUID)
	}
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

// inviteAcceptedMessage is the payload published by the invite service on InviteAcceptedSubject.
type inviteAcceptedMessage struct {
	InviteUID string `json:"invite_uid"`
	Username  string `json:"username"`
}

// HandleInviteAccepted processes an invite acceptance event from the invite service.
// It locates the project settings that own the invite via the mapping written at invite-send time,
// promotes the user from non-LFID (email-only) to LFID (username set, invite cleared), persists
// the update, deletes the consumed mapping, and re-indexes.
func (s *ProjectsService) HandleInviteAccepted(ctx context.Context, msg domain.Message) error {
	var event inviteAcceptedMessage
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to unmarshal invite_accepted event", constants.ErrKey, err)
		return nil
	}

	if event.InviteUID == "" || event.Username == "" {
		slog.WarnContext(ctx, "project_subscriber: invite_accepted event missing invite_uid or username — discarding",
			"invite_uid", event.InviteUID, "username", event.Username)
		return nil
	}

	// Look up the project UID from the mapping.
	projectUID, err := s.ProjectRepository.GetProjectUIDByInviteUID(ctx, event.InviteUID)
	if err != nil {
		if errors.Is(err, domain.ErrInviteMappingNotFound) {
			// No mapping means this invite belongs to another service — silently ignore.
			slog.DebugContext(ctx, "project_subscriber: invite not tracked by this service — ignoring",
				"invite_uid", event.InviteUID)
			return nil
		}
		slog.WarnContext(ctx, "project_subscriber: KV error looking up invite mapping",
			constants.ErrKey, err, "invite_uid", event.InviteUID)
		return nil
	}

	const maxPromoteRetries = 3
	var (
		settings *models.ProjectSettings
		promoted bool
	)
	for attempt := range maxPromoteRetries {
		var revision uint64
		var settingsErr error
		settings, revision, settingsErr = s.ProjectRepository.GetProjectSettingsWithRevision(ctx, projectUID)
		if settingsErr != nil {
			slog.WarnContext(ctx, "project_subscriber: failed to read settings for invite acceptance",
				constants.ErrKey, settingsErr, "project_uid", projectUID, "invite_uid", event.InviteUID)
			return nil
		}

		promoted = false
		for _, slice := range []*[]models.UserInfo{&settings.Writers, &settings.Auditors, &settings.MeetingCoordinators} {
			for i := range *slice {
				if (*slice)[i].Invite != nil && (*slice)[i].Invite.UID == event.InviteUID {
					(*slice)[i].Username = event.Username
					(*slice)[i].Invite = nil
					promoted = true
				}
			}
		}

		if !promoted {
			slog.WarnContext(ctx, "project_subscriber: invite UID not found in any role slice — stale mapping, cleaning up",
				"invite_uid", event.InviteUID, "project_uid", projectUID)
			if delErr := s.ProjectRepository.DeleteInviteMapping(ctx, event.InviteUID); delErr != nil {
				slog.WarnContext(ctx, "project_subscriber: failed to delete stale invite mapping", constants.ErrKey, delErr, "invite_uid", event.InviteUID)
			}
			return nil
		}

		updateErr := s.ProjectRepository.UpdateProjectSettings(ctx, settings, revision)
		if updateErr == nil {
			break
		}
		if !errors.Is(updateErr, domain.ErrRevisionMismatch) || attempt == maxPromoteRetries-1 {
			slog.WarnContext(ctx, "project_subscriber: failed to update settings after invite acceptance",
				constants.ErrKey, updateErr, "project_uid", projectUID, "invite_uid", event.InviteUID)
			return nil
		}
		slog.DebugContext(ctx, "project_subscriber: revision mismatch promoting invite — retrying",
			"attempt", attempt+1, "invite_uid", event.InviteUID)
	}

	if delErr := s.ProjectRepository.DeleteInviteMapping(ctx, event.InviteUID); delErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to delete invite mapping after acceptance",
			constants.ErrKey, delErr, "invite_uid", event.InviteUID)
	}

	slog.InfoContext(ctx, "project_subscriber: invite accepted — promoted user from non-LFID to LFID",
		"project_uid", projectUID, "invite_uid", event.InviteUID, "username", event.Username)

	indexMsg := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionUpdated,
		Data:           *settings,
		IndexingConfig: settings.IndexingConfig(projectUID),
	}
	if indexErr := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, indexMsg, false); indexErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to reindex project settings after invite acceptance",
			constants.ErrKey, indexErr, "project_uid", projectUID)
	}

	return nil
}

// wasInvitedInOldSettings returns true if the given email was already present in old settings
// as an email-only (Username == "") entry — meaning the user was previously invited and is now
// being promoted to LFID. Used to suppress redundant notification emails on promotion.
func wasInvitedInOldSettings(email string, old events.ProjectSettings) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, u := range append(append(old.Writers, old.Auditors...), old.MeetingCoordinators...) {
		if u.Username == "" && strings.ToLower(strings.TrimSpace(u.Email)) == normalized {
			return true
		}
	}
	return false
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
