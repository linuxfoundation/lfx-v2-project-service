// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
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

// changeKind classifies a per-user role delta between two settings snapshots.
type changeKind int

const (
	changeAdded   changeKind = iota // user is new to the project
	changeChanged                   // user's role set changed but they remain on the project
	changeRemoved                   // user was fully removed from the project
)

// userChange describes the role delta for a single user across a settings update.
type userChange struct {
	User     events.UserInfo // freshest snapshot (new settings if present, else old)
	OldRoles []string        // ordered: Writer, Auditor, Meeting Coordinator
	NewRoles []string
	Kind     changeKind
}

// HandleProjectSettingsUpdated handles project_settings.updated events and sends
// notification emails when users are added, have their roles changed, or are removed.
//
// LFID users (Username set) receive direct emails via the email service.
// Non-LFID users (email-only) receive invites for new roles via the invite service;
// removals for non-LFID users are silently skipped.
// Errors from individual sends are logged but never returned — the handler is best-effort.
func (s *ProjectsService) HandleProjectSettingsUpdated(ctx context.Context, msg domain.Message) error {
	var event events.ProjectSettingsUpdatedMessage
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to unmarshal project settings updated event", constants.ErrKey, err)
		return nil
	}

	changes := diffUserChanges(event.OldSettings, event.NewSettings)
	slog.DebugContext(ctx, "project_subscriber: received project_settings.updated event",
		"project_uid", event.ProjectUID, "change_count", len(changes))
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		for i, c := range changes {
			slog.DebugContext(ctx, "project_subscriber: user change detail",
				"project_uid", event.ProjectUID,
				"index", i,
				"kind", c.Kind,
				"username", c.User.Username,
				"email", c.User.Email,
				"old_roles", c.OldRoles,
				"new_roles", c.NewRoles,
			)
		}
	}
	if len(changes) == 0 {
		return nil
	}

	projectBase, err := s.ProjectRepository.GetProjectBase(ctx, event.ProjectUID)
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to load project", constants.ErrKey, err, "project_uid", event.ProjectUID)
		return nil
	}

	projectURL := buildProjectURL(s.Config.LFXSelfServeBaseURL, projectBase.Slug)
	inviterName := s.resolveActorDisplayName(ctx, event.Actor)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, change := range changes {
		g.Go(func() error {
			if change.User.Email == "" {
				slog.WarnContext(gctx, "project_subscriber: skipping notification — recipient has no email address",
					"change_kind", change.Kind, "project_uid", event.ProjectUID)
				return nil
			}

			recipientName := change.User.Name
			if recipientName == "" {
				recipientName = change.User.Username
			}
			if recipientName == "" {
				recipientName = change.User.Email
			}

			if change.User.Username == "" {
				// Non-LFID: route new roles through the invite service; skip removals.
				return s.handleNonLFIDChange(gctx, event.ProjectUID, projectBase.Name, change, recipientName, inviterName, projectURL)
			}

			// LFID user: send direct notification email.
			return s.handleLFIDChange(gctx, event.ProjectUID, projectBase.Name, change, recipientName, inviterName, projectURL)
		})
	}

	_ = g.Wait()
	return nil
}

// handleLFIDChange sends the appropriate email for a user who has an LFID.
func (s *ProjectsService) handleLFIDChange(ctx context.Context, projectUID, projectName string, change userChange, recipientName, inviterName, projectURL string) error {
	if !s.Config.EmailsEnabled {
		slog.DebugContext(ctx, "project_subscriber: skipping email — EMAILS_ENABLED is false",
			"project_uid", projectUID, "change_kind", change.Kind)
		return nil
	}
	switch change.Kind {
	case changeAdded:
		return s.sendRoleNotificationEmail(ctx, projectUID, projectName, change.NewRoles, change.User.Email, recipientName, inviterName, projectURL)
	case changeChanged:
		// Suppress email when the only change is gaining or losing a subordinate role
		// (Auditor, Meeting Coordinator) while Writer is held in both old and new — the
		// user's visible Manage access is unchanged.
		if isWriterSupersededNoOp(change.OldRoles, change.NewRoles) {
			slog.DebugContext(ctx, "project_subscriber: skipping role-changed email — gaining View on top of Manage is a no-op",
				"project_uid", projectUID, "old_roles", change.OldRoles, "new_roles", change.NewRoles)
			return nil
		}
		// Suppress email when a subordinate-role swap leaves the visible display identical
		// (e.g. Writer+Auditor → Writer+Meeting Coordinator both collapse to "Manage").
		if rolesEqual(rolesForDisplay(change.OldRoles), rolesForDisplay(change.NewRoles)) {
			slog.DebugContext(ctx, "project_subscriber: skipping role-changed email — display roles unchanged after collapsing",
				"project_uid", projectUID, "old_roles", change.OldRoles, "new_roles", change.NewRoles)
			return nil
		}
		return s.sendRoleChangedEmail(ctx, projectUID, projectName, change.OldRoles, change.NewRoles, change.User.Email, recipientName, inviterName, projectURL)
	case changeRemoved:
		return s.sendRoleRemovedEmail(ctx, projectUID, projectName, change.OldRoles, change.User.Email, recipientName, inviterName)
	}
	return nil
}

// handleNonLFIDChange sends invites for any newly-gained roles; removals are silently skipped.
func (s *ProjectsService) handleNonLFIDChange(ctx context.Context, projectUID, projectName string, change userChange, recipientName, inviterName, projectURL string) error {
	if !s.Config.InvitesEnabled {
		slog.DebugContext(ctx, "project_subscriber: skipping invite — INVITES_ENABLED is false",
			"project_uid", projectUID, "change_kind", change.Kind)
		return nil
	}
	if change.Kind == changeRemoved {
		slog.DebugContext(ctx, "project_subscriber: skipping removal notification for non-LFID user",
			"project_uid", projectUID)
		return nil
	}

	// For Added: send an invite for every new role.
	// For Changed: send an invite only for roles that are new (delta), not ones already held.
	// Skip entirely when the only new roles are View-level while the user already holds Manage.
	var rolesToInvite []string
	if change.Kind == changeAdded {
		rolesToInvite = change.NewRoles
	} else {
		if isWriterSupersededNoOp(change.OldRoles, change.NewRoles) {
			slog.DebugContext(ctx, "project_subscriber: skipping invite — gaining View on top of Manage is a no-op",
				"project_uid", projectUID)
			return nil
		}
		rolesToInvite = setDiffRoles(change.NewRoles, change.OldRoles)
	}

	// Deduplicate by mapped invite role before sending — Writer and Meeting Coordinator
	// both map to Manage, so having both in rolesToInvite would otherwise trigger two
	// invites for the same effective access level.
	seenInviteRole := make(map[string]bool, len(rolesToInvite))
	for _, role := range rolesToInvite {
		inviteRole := mapRoleToInviteRole(role)
		if inviteRole == "" || seenInviteRole[inviteRole] {
			continue
		}
		seenInviteRole[inviteRole] = true
		if err := s.sendInvite(ctx, projectUID, projectName, role, change.User.Email, recipientName, inviterName, projectURL); err != nil {
			return err
		}
	}
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
		"role", role, "invite_role", inviteRole, "project_uid", projectUID)

	sendCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
	defer cancel()

	result, err := s.MessageBuilder.SendInviteRequest(sendCtx, inviteapi.SendInviteRequest{
		Recipient: &inviteapi.Recipient{
			Email: recipientEmail,
			Name:  recipientName,
		},
		Inviter: &inviteapi.Inviter{
			Name: inviterName,
		},
		Resource: &inviteapi.Resource{
			UID:  projectUID,
			Name: projectName,
			Type: "project",
		},
		Role:           inviteRole,
		ReturnURL:      deepLinkURL,
		ExpirationDays: 30,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to send invite request",
			constants.ErrKey, err, "role", role, "project_uid", projectUID)
		return nil
	}
	slog.InfoContext(ctx, "project_subscriber: invite sent",
		"role", role, "project_uid", projectUID, "invite_uid", result.InviteUID)

	return nil
}

// sendRoleNotificationEmail sends a direct "you were added" notification email via
// the email service for a user who already has an LFID.
func (s *ProjectsService) sendRoleNotificationEmail(ctx context.Context, projectUID, projectName string, roles []string, recipientEmail, recipientName, inviterName, projectURL string) error {
	subject, html, text, err := email.RenderProjectRoleNotification(email.ProjectRoleNotificationData{
		RecipientName: recipientName,
		ProjectName:   projectName,
		Roles:         rolesForDisplay(roles),
		ProjectURL:    projectURL,
		InviterName:   inviterName,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to render role notification email template",
			constants.ErrKey, err, "project_uid", projectUID)
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
			constants.ErrKey, sendErr, "project_uid", projectUID)
	} else {
		slog.DebugContext(ctx, "project_subscriber: sent role notification email", "project_uid", projectUID)
	}
	return nil
}

// sendRoleChangedEmail sends a "your role was updated" notification email for a user
// whose role set changed but who remains on the project.
func (s *ProjectsService) sendRoleChangedEmail(ctx context.Context, projectUID, projectName string, oldRoles, newRoles []string, recipientEmail, recipientName, inviterName, projectURL string) error {
	subject, html, text, err := email.RenderProjectRoleChanged(email.ProjectRoleChangedData{
		RecipientName: recipientName,
		ProjectName:   projectName,
		OldRoles:      rolesForDisplay(oldRoles),
		NewRoles:      rolesForDisplay(newRoles),
		ProjectURL:    projectURL,
		InviterName:   inviterName,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to render role changed email template",
			constants.ErrKey, err, "project_uid", projectUID)
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
		slog.WarnContext(ctx, "project_subscriber: failed to send role changed email",
			constants.ErrKey, sendErr, "project_uid", projectUID)
	} else {
		slog.DebugContext(ctx, "project_subscriber: sent role changed email", "project_uid", projectUID)
	}
	return nil
}

// sendRoleRemovedEmail sends a "you have been removed" notification email for a user
// who no longer has any role on the project.
func (s *ProjectsService) sendRoleRemovedEmail(ctx context.Context, projectUID, projectName string, oldRoles []string, recipientEmail, recipientName, inviterName string) error {
	subject, html, text, err := email.RenderProjectRoleRemoved(email.ProjectRoleRemovedData{
		RecipientName: recipientName,
		ProjectName:   projectName,
		OldRoles:      rolesForDisplay(oldRoles),
		InviterName:   inviterName,
	})
	if err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to render role removed email template",
			constants.ErrKey, err, "project_uid", projectUID)
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
		slog.WarnContext(ctx, "project_subscriber: failed to send role removed email",
			constants.ErrKey, sendErr, "project_uid", projectUID)
	} else {
		slog.DebugContext(ctx, "project_subscriber: sent role removed email", "project_uid", projectUID)
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

// HandleInviteAccepted processes an invite acceptance event published by the invite service.
// It scans all project settings for email-only user entries matching the recipient email and
// promotes them to full LFID users (username set, invite cleared). This reconciles every
// project the accepted user was invited to, regardless of which resource triggered the event.
//
// TODO: replace the full-scan with an email → [project_uid] index lookup so we avoid reading
// every project's settings on each acceptance event.
func (s *ProjectsService) HandleInviteAccepted(ctx context.Context, msg domain.Message) error {
	var event inviteapi.InviteServiceAcceptedEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to unmarshal invite_accepted event", constants.ErrKey, err)
		return nil
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(event.Recipient.Email))
	if event.UID == "" || event.AcceptedBy == "" || normalizedEmail == "" {
		slog.WarnContext(ctx, "project_subscriber: invite_accepted event missing required fields — discarding",
			"invite_uid", event.UID, "has_accepted_by", event.AcceptedBy != "",
			"has_recipient_email", normalizedEmail != "")
		return nil
	}

	// Scan all project settings for email-only entries that match the recipient.
	listCtx, listCancel := context.WithTimeout(ctx, notificationTimeout)
	allSettings, listErr := s.ProjectRepository.ListAllProjectsSettings(listCtx)
	listCancel()
	if listErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to list project settings for invite reconciliation",
			constants.ErrKey, listErr, "invite_uid", event.UID)
		return nil
	}

	for _, candidate := range allSettings {
		if !projectSettingsHasEmailOnlyEntry(candidate, normalizedEmail, event.Role) {
			continue
		}
		projectUID := candidate.UID
		promoteCtx, promoteCancel := context.WithTimeout(ctx, notificationTimeout)
		s.promoteInvitedUserInProjectSettings(promoteCtx, projectUID, normalizedEmail, event.AcceptedBy, event.UID, event.Role)
		promoteCancel()
	}

	return nil
}

// projectSettingsHasEmailOnlyEntry reports whether settings contain at least one
// email-only (non-LFID) entry whose email matches normalizedEmail, considering only
// the role-appropriate slices.
func projectSettingsHasEmailOnlyEntry(s *models.ProjectSettings, normalizedEmail, role string) bool {
	for _, slice := range projectRoleSlices(s, role) {
		for _, u := range slice {
			if u.Username == "" && strings.ToLower(strings.TrimSpace(u.Email)) == normalizedEmail {
				return true
			}
		}
	}
	return false
}

// projectRoleSlices returns the settings slices to scan/update for a given invite role.
// "Manage" → Writers + MeetingCoordinators; "View" → Auditors only; unknown → all three.
func projectRoleSlices(s *models.ProjectSettings, role string) [][]models.UserInfo {
	switch role {
	case string(inviteapi.InviteRoleManage):
		return [][]models.UserInfo{s.Writers, s.MeetingCoordinators}
	case string(inviteapi.InviteRoleView):
		return [][]models.UserInfo{s.Auditors}
	default:
		return [][]models.UserInfo{s.Writers, s.Auditors, s.MeetingCoordinators}
	}
}

// projectRoleSlicePtrs returns pointer-to-slice refs matching projectRoleSlices, for mutation.
func projectRoleSlicePtrs(s *models.ProjectSettings, role string) []*[]models.UserInfo {
	switch role {
	case string(inviteapi.InviteRoleManage):
		return []*[]models.UserInfo{&s.Writers, &s.MeetingCoordinators}
	case string(inviteapi.InviteRoleView):
		return []*[]models.UserInfo{&s.Auditors}
	default:
		return []*[]models.UserInfo{&s.Writers, &s.Auditors, &s.MeetingCoordinators}
	}
}

// promoteInvitedUserInProjectSettings promotes all email-only entries matching normalizedEmail
// in the given project's settings to full LFID users. It retries on revision conflicts.
func (s *ProjectsService) promoteInvitedUserInProjectSettings(ctx context.Context, projectUID, normalizedEmail, username, inviteUID, role string) {
	const maxRetries = 3
	for attempt := range maxRetries {
		settings, revision, err := s.ProjectRepository.GetProjectSettingsWithRevision(ctx, projectUID)
		if err != nil {
			slog.WarnContext(ctx, "project_subscriber: failed to read settings for invite promotion",
				constants.ErrKey, err, "project_uid", projectUID, "invite_uid", inviteUID)
			return
		}

		promoted := false
		for _, slice := range projectRoleSlicePtrs(settings, role) {
			for i := range *slice {
				if (*slice)[i].Username == "" && strings.ToLower(strings.TrimSpace((*slice)[i].Email)) == normalizedEmail {
					(*slice)[i].Username = username
					(*slice)[i].Invite = nil
					promoted = true
				}
			}
		}

		if !promoted {
			// Race: another handler already promoted this entry between the scan and now.
			slog.DebugContext(ctx, "project_subscriber: email-only entry already promoted — skipping",
				"project_uid", projectUID, "invite_uid", inviteUID)
			return
		}

		updateErr := s.ProjectRepository.UpdateProjectSettings(ctx, settings, revision)
		if updateErr == nil {
			slog.InfoContext(ctx, "project_subscriber: invite accepted — promoted user from non-LFID to LFID",
				"project_uid", projectUID, "invite_uid", inviteUID, "username", username)
			indexMsg := indexerTypes.IndexerMessageEnvelope{
				Action:         indexerConstants.ActionUpdated,
				Data:           *settings,
				IndexingConfig: settings.IndexingConfig(projectUID),
			}
			if indexErr := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, indexMsg, false); indexErr != nil {
				slog.WarnContext(ctx, "project_subscriber: failed to reindex project settings after invite acceptance",
					constants.ErrKey, indexErr, "project_uid", projectUID)
			}
			return
		}
		if !errors.Is(updateErr, domain.ErrRevisionMismatch) || attempt == maxRetries-1 {
			slog.WarnContext(ctx, "project_subscriber: failed to update settings after invite acceptance",
				constants.ErrKey, updateErr, "project_uid", projectUID, "invite_uid", inviteUID)
			return
		}
		slog.DebugContext(ctx, "project_subscriber: revision mismatch promoting invite — retrying",
			"attempt", attempt+1, "project_uid", projectUID, "invite_uid", inviteUID)
	}
}

// buildProjectURL constructs the deep-link URL for a project's overview page.
func buildProjectURL(baseURL, slug string) string {
	base := strings.TrimRight(baseURL, "/") + "/project/overview"
	if slug != "" {
		return base + "?project=" + url.QueryEscape(slug)
	}
	return base
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

// diffUserChanges returns the per-user role delta between two settings snapshots.
// Each entry describes a single user and whether they were added, had their role
// set changed, or were fully removed.  Users whose role set is identical across
// both snapshots are omitted.  Role order in OldRoles / NewRoles is stable:
// Writer, Auditor, Meeting Coordinator.
func diffUserChanges(old, new events.ProjectSettings) []userChange {
	type entry struct {
		user  events.UserInfo
		roles []string
	}

	buildMap := func(settings events.ProjectSettings) (primary map[string]entry, allKeys map[string]string) {
		primary = make(map[string]entry)
		allKeys = make(map[string]string)

		add := func(u events.UserInfo, role string) {
			keys := memberKeys(u)
			if len(keys) == 0 {
				return
			}

			// Find the canonical primary key for this user, resolving across identity
			// shapes (e.g. email-only entry followed by username+email for the same person).
			canonKey := ""
			for _, k := range keys {
				if pk, ok := allKeys[k]; ok {
					canonKey = pk
					break
				}
			}
			if canonKey == "" {
				canonKey = keys[0]
			}

			e := primary[canonKey]
			// Prefer the most complete identity record: take the new entry only if it
			// has a Username or the stored record has none yet.  This prevents an
			// email-only invite entry (Username="") from wiping out a Username+Email
			// entry seen earlier in a different role slice.
			if u.Username != "" || e.user.Username == "" {
				e.user = u
			}
			// Guard against duplicate user entries within the same role slice.
			alreadyHas := false
			for _, r := range e.roles {
				if r == role {
					alreadyHas = true
					break
				}
			}
			if !alreadyHas {
				e.roles = append(e.roles, role)
			}
			primary[canonKey] = e
			for _, k := range keys {
				allKeys[k] = canonKey
			}
		}

		for _, u := range settings.Writers {
			add(u, roleWriter)
		}
		for _, u := range settings.Auditors {
			add(u, roleAuditor)
		}
		for _, u := range settings.MeetingCoordinators {
			add(u, roleMeetingCoordinator)
		}
		return
	}

	oldPrimary, oldAllKeys := buildMap(old)
	newPrimary, newAllKeys := buildMap(new)

	var changes []userChange
	matchedOldKeys := make(map[string]bool, len(newPrimary))

	for _, newEntry := range newPrimary {
		// Resolve which old primary key (if any) corresponds to this new user.
		oldCanon := ""
		for _, k := range memberKeys(newEntry.user) {
			if pk, ok := oldAllKeys[k]; ok {
				oldCanon = pk
				break
			}
		}

		if oldCanon == "" {
			changes = append(changes, userChange{
				User:     newEntry.user,
				NewRoles: newEntry.roles,
				Kind:     changeAdded,
			})
			continue
		}
		matchedOldKeys[oldCanon] = true

		oldEntry := oldPrimary[oldCanon]
		if rolesEqual(oldEntry.roles, newEntry.roles) {
			continue // no change
		}
		changes = append(changes, userChange{
			User:     newEntry.user,
			OldRoles: oldEntry.roles,
			NewRoles: newEntry.roles,
			Kind:     changeChanged,
		})
	}

	// Users present in old but absent from new are fully removed.
	for oldCanon, oldEntry := range oldPrimary {
		if matchedOldKeys[oldCanon] {
			continue
		}
		// Double-check via newAllKeys in case the resolution above missed a key.
		found := false
		for _, k := range memberKeys(oldEntry.user) {
			if _, ok := newAllKeys[k]; ok {
				found = true
				break
			}
		}
		if !found {
			changes = append(changes, userChange{
				User:     oldEntry.user,
				OldRoles: oldEntry.roles,
				Kind:     changeRemoved,
			})
		}
	}

	return changes
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
		keys = append(keys, "email:"+strings.ToLower(strings.TrimSpace(u.Email)))
	}
	return keys
}

// rolesEqual reports whether two role slices contain the same elements in the same order.
func rolesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// setDiffRoles returns roles present in a but not in b.
func setDiffRoles(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, r := range b {
		bSet[r] = struct{}{}
	}
	var diff []string
	for _, r := range a {
		if _, ok := bSet[r]; !ok {
			diff = append(diff, r)
		}
	}
	return diff
}

// hasWriterRole reports whether roles includes the Writer role, which supersedes all other roles.
func hasWriterRole(roles []string) bool {
	for _, r := range roles {
		if r == roleWriter {
			return true
		}
	}
	return false
}

// isWriterSupersededNoOp reports whether Writer is present in both old and new roles and the
// change is a purely additive or purely subtractive adjustment of subordinate roles (Auditor or
// Meeting Coordinator) that Writer already supersedes.  Swaps (simultaneously gaining one
// subordinate while losing another) are not suppressed — the visible role set still changed.
func isWriterSupersededNoOp(oldRoles, newRoles []string) bool {
	if !hasWriterRole(oldRoles) || !hasWriterRole(newRoles) {
		return false
	}
	gained := setDiffRoles(newRoles, oldRoles)
	lost := setDiffRoles(oldRoles, newRoles)
	// A swap of subordinate roles is still a meaningful change.
	if len(gained) > 0 && len(lost) > 0 {
		return false
	}
	delta := make([]string, 0, len(gained)+len(lost))
	delta = append(delta, gained...)
	delta = append(delta, lost...)
	if len(delta) == 0 {
		return false
	}
	for _, r := range delta {
		if r != roleAuditor && r != roleMeetingCoordinator {
			return false
		}
	}
	return true
}

// roleDisplayName maps an internal role name to its user-facing display name.
// Writer → "Manage", Auditor → "View", Meeting Coordinator stays as-is.
func roleDisplayName(role string) string {
	switch role {
	case roleWriter:
		return "Manage"
	case roleAuditor:
		return "View"
	default:
		return role
	}
}

// rolesForDisplay converts a slice of internal role names to deduplicated display names
// ("Manage", "Meeting Coordinator", "View"), then returns just ["Manage"] when Writer is
// present, since Writer supersedes both Meeting Coordinator and View.
// When no Writer, Meeting Coordinator and View are shown independently.  Order follows input.
func rolesForDisplay(roles []string) []string {
	seen := make(map[string]bool, len(roles))
	result := make([]string, 0, len(roles))
	for _, r := range roles {
		d := roleDisplayName(r)
		if !seen[d] {
			seen[d] = true
			result = append(result, d)
		}
	}
	if seen["Manage"] {
		return []string{"Manage"}
	}
	return result
}
