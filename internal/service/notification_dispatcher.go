// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service/email"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"golang.org/x/sync/errgroup"
)

// notificationTimeout caps blocking outbound calls: email-service request/reply,
// invite-service request/reply, and auth-service actor name lookup.
const notificationTimeout = 5 * time.Second

// NotificationDispatcher sends role-change notifications (emails and invites) for a
// settings-update event. It owns all email rendering, invite deduplication, LFID /
// non-LFID routing, and feature-flag suppression behind a single Dispatch call.
//
// Callers supply only the project identity (UID/name/URL), the actor, and the computed
// change list; all outbound RPC complexity is internal. Dispatch is best-effort —
// individual send failures are logged and swallowed.
type NotificationDispatcher struct {
	builder  domain.MessageBuilder
	resolver *UserResolver
	emails   bool
	invites  bool
}

// NewNotificationDispatcher returns a dispatcher backed by builder and resolver.
// emailsEnabled and invitesEnabled mirror the EMAILS_ENABLED / INVITES_ENABLED flags.
func NewNotificationDispatcher(builder domain.MessageBuilder, resolver *UserResolver, emailsEnabled, invitesEnabled bool) *NotificationDispatcher {
	return &NotificationDispatcher{
		builder:  builder,
		resolver: resolver,
		emails:   emailsEnabled,
		invites:  invitesEnabled,
	}
}

// Dispatch sends notifications for every user-role change in a settings update.
// It fans out over changes concurrently (up to 5 goroutines), resolves the actor
// display name internally, and routes each change to the LFID or non-LFID send path.
// Errors from individual sends are logged and swallowed; Dispatch always returns nil.
func (d *NotificationDispatcher) Dispatch(ctx context.Context, projectUID, projectName, projectURL string, actor events.Actor, changes []userChange) error {
	inviterName := d.resolver.ResolveDisplayName(ctx, actor)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, change := range changes {
		g.Go(func() error {
			if change.User.Email == "" {
				slog.WarnContext(gctx, "project_subscriber: skipping notification — recipient has no email address",
					"change_kind", change.Kind, "project_uid", projectUID)
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
				return d.handleNonLFIDChange(gctx, projectUID, projectName, change, recipientName, inviterName, projectURL)
			}
			return d.handleLFIDChange(gctx, projectUID, projectName, change, recipientName, inviterName, projectURL)
		})
	}

	_ = g.Wait()
	return nil
}

// handleLFIDChange sends the appropriate email for a user who has an LFID.
func (d *NotificationDispatcher) handleLFIDChange(ctx context.Context, projectUID, projectName string, change userChange, recipientName, inviterName, projectURL string) error {
	if !d.emails {
		slog.DebugContext(ctx, "project_subscriber: skipping email — EMAILS_ENABLED is false",
			"project_uid", projectUID, "change_kind", change.Kind)
		return nil
	}
	switch change.Kind {
	case changeAdded:
		return d.sendRoleNotificationEmail(ctx, projectUID, projectName, change.NewRoles, change.User.Email, recipientName, inviterName, projectURL)
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
		return d.sendRoleChangedEmail(ctx, projectUID, projectName, change.OldRoles, change.NewRoles, change.User.Email, recipientName, inviterName, projectURL)
	case changeRemoved:
		return d.sendRoleRemovedEmail(ctx, projectUID, projectName, change.OldRoles, change.User.Email, recipientName, inviterName)
	}
	return nil
}

// handleNonLFIDChange sends invites for any newly-gained roles; removals are silently skipped.
func (d *NotificationDispatcher) handleNonLFIDChange(ctx context.Context, projectUID, projectName string, change userChange, recipientName, inviterName, projectURL string) error {
	if !d.invites {
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
	if len(rolesToInvite) == 0 {
		return nil
	}

	// Check whether the recipient already has an LFX account so the invite service
	// can choose the correct email template. Best-effort: any lookup error leaves
	// recipientHasAccount false, which falls back to the new-user template safely.
	// Lookup runs after role resolution so we skip the RPC when no invites will be sent.
	recipientHasAccount := false
	acctCtx, acctCancel := context.WithTimeout(ctx, notificationTimeout)
	if username, lookupErr := d.resolver.UsernameByEmail(acctCtx, strings.TrimSpace(change.User.Email)); lookupErr == nil && username != "" {
		recipientHasAccount = true
	} else if lookupErr != nil && !errors.Is(lookupErr, domain.ErrUserNotFound) {
		slog.WarnContext(ctx, "project_subscriber: LFID lookup failed; falling back to new-user invite template",
			"project_uid", projectUID, constants.ErrKey, lookupErr)
	}
	acctCancel()

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
		if err := d.sendInvite(ctx, projectUID, projectName, role, change.User.Email, recipientName, inviterName, projectURL, recipientHasAccount); err != nil {
			return err
		}
	}
	return nil
}

// sendInvite sends a send-invite request to the invite service for a non-LFID user.
// recipientHasAccount controls which email template the invite service renders:
// true → existing-account path ("Accept invitation"); false → new-user path ("Accept invitation & create account").
func (d *NotificationDispatcher) sendInvite(ctx context.Context, projectUID, projectName, role, recipientEmail, recipientName, inviterName, deepLinkURL string, recipientHasAccount bool) error {
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

	result, err := d.builder.SendInviteRequest(sendCtx, inviteapi.SendInviteRequest{
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
		Role:                inviteRole,
		ReturnURL:           deepLinkURL,
		ExpirationDays:      30,
		RecipientHasAccount: recipientHasAccount,
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
func (d *NotificationDispatcher) sendRoleNotificationEmail(ctx context.Context, projectUID, projectName string, roles []string, recipientEmail, recipientName, inviterName, projectURL string) error {
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

	sendErr := d.builder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
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
func (d *NotificationDispatcher) sendRoleChangedEmail(ctx context.Context, projectUID, projectName string, oldRoles, newRoles []string, recipientEmail, recipientName, inviterName, projectURL string) error {
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

	sendErr := d.builder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
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
func (d *NotificationDispatcher) sendRoleRemovedEmail(ctx context.Context, projectUID, projectName string, oldRoles []string, recipientEmail, recipientName, inviterName string) error {
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

	sendErr := d.builder.SendEmailRequest(sendCtx, emailapi.SendEmailRequest{
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

// mapRoleToInviteRole converts a project-service role string to the invite service's
// role vocabulary. Returns an empty string for unrecognised roles (caller skips invite).
//
// Mapping:
//   - Writer              → Manage
//   - Auditor             → View
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
// Meeting Coordinator) that Writer already supersedes. Swaps (simultaneously gaining one
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
// When no Writer, Meeting Coordinator and View are shown independently. Order follows input.
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
