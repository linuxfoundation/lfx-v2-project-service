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

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

// settingsScanTimeout caps ListAllProjectsSettings and per-project reconciliation work.
// Unlike notificationTimeout (single RPC), a full project-settings bucket scan can require
// many sequential KV reads under load.
const settingsScanTimeout = 2 * time.Minute

// scrubMaxRetries is the number of attempts for settings KV writes and indexer/FGA publishes
// after a successful scrub. Retries are independent of the username match so a transient
// conflict or NATS failure does not leave access tuples stale after settings were scrubbed.
const scrubMaxRetries = 4

// serviceAuthBearer is the static JWT audience token used for background NATS handler
// side effects (indexer/FGA) that have no originating HTTP request context.
// Pattern matches lfx-v2-committee-service message_handler.go.
const serviceAuthBearer = "Bearer lfx-v2-project-service"

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
	return s.Dispatcher.Dispatch(ctx, event.ProjectUID, projectBase.Name, projectURL, event.Actor, changes)
}

// HandleInviteAccepted processes an invite acceptance event published by the invite service.
// It scans all project settings for email-only user entries matching the recipient email and
// promotes them to full LFID users (username set, invite cleared). This reconciles every
// project the accepted user was invited to, regardless of which resource triggered the event.
//
// Note: accepting a single invite reconciles every project where the same email has a
// pending email-only entry for the same role, not only the project that issued the invite.
// This is intentional and idempotent.
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
	validRole := event.Role == string(inviteapi.InviteRoleManage) || event.Role == string(inviteapi.InviteRoleView)
	if event.UID == "" || event.AcceptedBy == "" || normalizedEmail == "" || !validRole {
		slog.WarnContext(ctx, "project_subscriber: invite_accepted event missing or unrecognized required fields — discarding",
			"invite_uid", event.UID, "has_accepted_by", event.AcceptedBy != "",
			"has_recipient_email", normalizedEmail != "", "role", event.Role)
		return nil
	}

	// Scan all project settings for email-only entries that match the recipient.
	listCtx, listCancel := context.WithTimeout(ctx, settingsScanTimeout)
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
		promoteCtx, promoteCancel := context.WithTimeout(ctx, settingsScanTimeout)
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

// projectRoleSlicePtrs returns pointer-to-slice refs for mutation for a given invite role.
// "Manage" → Writers + MeetingCoordinators; "View" → Auditors only; unknown → nil (fail closed).
func projectRoleSlicePtrs(s *models.ProjectSettings, role string) []*[]models.UserInfo {
	switch role {
	case string(inviteapi.InviteRoleManage):
		return []*[]models.UserInfo{&s.Writers, &s.MeetingCoordinators}
	case string(inviteapi.InviteRoleView):
		return []*[]models.UserInfo{&s.Auditors}
	default:
		return nil // unknown/unrecognized role — fail closed, do not promote into any slice
	}
}

// projectRoleSlices returns the settings slices to scan for a given invite role.
// Derived from projectRoleSlicePtrs so role mappings cannot drift between the two.
func projectRoleSlices(s *models.ProjectSettings, role string) [][]models.UserInfo {
	ptrs := projectRoleSlicePtrs(s, role)
	if ptrs == nil {
		return nil
	}
	slices := make([][]models.UserInfo, len(ptrs))
	for i, p := range ptrs {
		slices[i] = *p
	}
	return slices
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
			if indexErr := s.MessageBuilder.SendIndexerMessage(ctxWithServiceAuth(ctx), constants.IndexProjectSettingsSubject, indexMsg, false); indexErr != nil {
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

// HandleUserDeleted scrubs the deleted user's username from project settings.
// Best-effort: partial failures are logged but do not block the overall scrub.
// Unlike committee data, project settings have no separate member records — only
// the settings writers/auditors/meeting coordinators and named role fields are updated.
func (s *ProjectsService) HandleUserDeleted(ctx context.Context, msg domain.Message) error {
	var event events.V1UserDeletedEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to unmarshal user.deleted event", constants.ErrKey, err)
		return nil
	}
	if strings.TrimSpace(event.Username) == "" {
		slog.WarnContext(ctx, "project_subscriber: user.deleted event missing username — nothing to scrub")
		return nil
	}

	slog.InfoContext(ctx, "project_subscriber: scrubbing deleted user's username from project settings")

	listCtx, listCancel := context.WithTimeout(ctx, settingsScanTimeout)
	allSettings, listErr := s.ProjectRepository.ListAllProjectsSettings(listCtx)
	listCancel()
	if listErr != nil {
		slog.WarnContext(ctx, "project_subscriber: failed to list project settings for username scrub",
			constants.ErrKey, listErr)
		return nil
	}

	for _, candidate := range allSettings {
		if !projectSettingsHasUsername(candidate, event.Username) {
			continue
		}
		scrubCtx, scrubCancel := context.WithTimeout(ctx, settingsScanTimeout)
		s.scrubProjectSettingsUsername(scrubCtx, candidate.UID, event.Username, event.Email)
		scrubCancel()
	}

	return nil
}

// usernameMatches reports whether storedUsername represents the same LFID as deletedUsername.
func usernameMatches(deletedUsername, storedUsername string) bool {
	deleted := strings.TrimSpace(deletedUsername)
	stored := strings.TrimSpace(storedUsername)
	if deleted == "" || stored == "" {
		return false
	}
	return strings.EqualFold(deleted, stored)
}

// projectSettingsHasUsername reports whether any role-bearing field in settings carries username.
func projectSettingsHasUsername(s *models.ProjectSettings, username string) bool {
	if s == nil {
		return false
	}
	for _, u := range s.Writers {
		if usernameMatches(username, u.Username) {
			return true
		}
	}
	for _, u := range s.Auditors {
		if usernameMatches(username, u.Username) {
			return true
		}
	}
	for _, u := range s.MeetingCoordinators {
		if usernameMatches(username, u.Username) {
			return true
		}
	}
	if s.ExecutiveDirector != nil && usernameMatches(username, s.ExecutiveDirector.Username) {
		return true
	}
	if s.ProgramManager != nil && usernameMatches(username, s.ProgramManager.Username) {
		return true
	}
	if s.OpportunityOwner != nil && usernameMatches(username, s.OpportunityOwner.Username) {
		return true
	}
	return false
}

// clearUsernameInSettings clears username on every matching entry in settings
// that still represents the deleted account. Returns true when at least one field was changed.
func (s *ProjectsService) clearUsernameInSettings(ctx context.Context, settings *models.ProjectSettings, username, deletedEmail string) bool {
	changed := false
	clearIfMatch := func(u *models.UserInfo) {
		if u == nil || !usernameMatches(username, u.Username) {
			return
		}
		if !s.shouldScrubSettingsUsername(ctx, *u, username, deletedEmail) {
			slog.DebugContext(ctx, "project_subscriber: skipping username scrub — email still maps to active LFID",
				"username", username, "email", u.Email)
			return
		}
		u.Username = ""
		changed = true
	}

	for i := range settings.Writers {
		clearIfMatch(&settings.Writers[i])
	}
	for i := range settings.Auditors {
		clearIfMatch(&settings.Auditors[i])
	}
	for i := range settings.MeetingCoordinators {
		clearIfMatch(&settings.MeetingCoordinators[i])
	}
	clearIfMatch(settings.ExecutiveDirector)
	clearIfMatch(settings.ProgramManager)
	clearIfMatch(settings.OpportunityOwner)
	return changed
}

// shouldScrubSettingsUsername reports whether a settings entry carrying deletedUsername should
// be cleared. When the deletion event carries an email, entries with a different non-empty email
// are treated as reuse and skipped; a matching entry email is a definitive identification and
// scrubs without auth lookup. When the event omits email, auth is consulted for entries that
// have an email so a reassigned LFID reused by a new account is not scrubbed.
//
// Entries without email scrub when the username matches only when the deletion event
// also omits email (M2M/legacy). When the event carries email, username-only entries are
// skipped because they cannot be verified as the deleted account.
func (s *ProjectsService) shouldScrubSettingsUsername(ctx context.Context, u models.UserInfo, deletedUsername, deletedEmail string) bool {
	entryEmail := strings.ToLower(strings.TrimSpace(u.Email))
	deletedEmailNorm := strings.ToLower(strings.TrimSpace(deletedEmail))
	if deletedEmailNorm != "" {
		if entryEmail == "" {
			return false
		}
		if entryEmail != deletedEmailNorm {
			return false
		}
		return true // definitive match — scrub without auth lookup
	}
	if entryEmail == "" {
		return true
	}
	if s.UserReader == nil {
		return true
	}

	lookupCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
	defer cancel()

	resolved, err := s.UserReader.UsernameByEmail(lookupCtx, entryEmail)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return true
		}
		slog.WarnContext(ctx, "project_subscriber: auth lookup failed during username scrub — skipping entry",
			constants.ErrKey, err)
		return false
	}
	return !usernameMatches(resolved, deletedUsername)
}

// scrubProjectSettingsUsername fetches settings for a single project, clears the
// username on any matching entry, persists, and reindexes. Retries on revision conflicts.
func (s *ProjectsService) scrubProjectSettingsUsername(ctx context.Context, projectUID, username, deletedEmail string) {
	for attempt := 0; attempt < scrubMaxRetries; attempt++ {
		settings, revision, err := s.ProjectRepository.GetProjectSettingsWithRevision(ctx, projectUID)
		if err != nil {
			slog.DebugContext(ctx, "project_subscriber: failed to get settings for username scrub — skipping",
				constants.ErrKey, err, "project_uid", projectUID)
			return
		}
		if settings == nil {
			return
		}

		if !s.clearUsernameInSettings(ctx, settings, username, deletedEmail) {
			return
		}

		updateErr := s.ProjectRepository.UpdateProjectSettings(ctx, settings, revision)
		if updateErr == nil {
			slog.InfoContext(ctx, "project_subscriber: cleared username from project settings",
				"project_uid", projectUID)
			s.publishProjectSettingsScrubSideEffects(ctx, projectUID)
			return
		}
		if !errors.Is(updateErr, domain.ErrRevisionMismatch) || attempt == scrubMaxRetries-1 {
			slog.WarnContext(ctx, "project_subscriber: failed to clear username from project settings",
				constants.ErrKey, updateErr, "project_uid", projectUID)
			return
		}
		slog.DebugContext(ctx, "project_subscriber: revision mismatch scrubbing username — retrying",
			"attempt", attempt+1, "project_uid", projectUID)
	}
}

// publishProjectSettingsScrubSideEffects reindexes scrubbed settings and refreshes OpenFGA
// access tuples. Each attempt reloads the current KV record before publishing; indexer and FGA
// projections are full-state and idempotent. ProjectSettingsUpdatedSubject is intentionally
// omitted to avoid role-change emails.
func (s *ProjectsService) publishProjectSettingsScrubSideEffects(ctx context.Context, projectUID string) {
	ctx = ctxWithServiceAuth(ctx)
	for attempt := 0; attempt < scrubMaxRetries; attempt++ {
		settings, _, err := s.ProjectRepository.GetProjectSettingsWithRevision(ctx, projectUID)
		if err != nil {
			if attempt == scrubMaxRetries-1 {
				slog.WarnContext(ctx, "project_subscriber: failed to reload settings for scrub side effects",
					constants.ErrKey, err, "project_uid", projectUID, "attempts", scrubMaxRetries)
				return
			}
			slog.DebugContext(ctx, "project_subscriber: retrying scrub side effects after settings reload failure",
				"attempt", attempt+1, "project_uid", projectUID)
			continue
		}
		if settings == nil {
			return
		}

		projectBase, baseErr := s.ProjectRepository.GetProjectBase(ctx, projectUID)
		if baseErr != nil {
			slog.WarnContext(ctx, "project_subscriber: failed to load project for FGA refresh after username scrub",
				constants.ErrKey, baseErr, "project_uid", projectUID)
			if attempt == scrubMaxRetries-1 {
				return
			}
			slog.DebugContext(ctx, "project_subscriber: retrying scrub side effects after project load failure",
				"attempt", attempt+1, "project_uid", projectUID)
			continue
		}

		indexMsg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionUpdated,
			Data:           *settings,
			IndexingConfig: settings.IndexingConfig(projectUID),
		}
		indexErr := s.MessageBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, indexMsg, false)

		fgaMsg := buildFGAUpdateAccessMessage(projectBase, settings)
		accessErr := s.MessageBuilder.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, fgaMsg)

		if indexErr == nil && accessErr == nil {
			return
		}

		if attempt == scrubMaxRetries-1 {
			if indexErr != nil {
				slog.WarnContext(ctx, "project_subscriber: failed to reindex project settings after username scrub",
					constants.ErrKey, indexErr, "project_uid", projectUID, "attempts", scrubMaxRetries)
			}
			if accessErr != nil {
				slog.WarnContext(ctx, "project_subscriber: failed to publish FGA update after username scrub",
					constants.ErrKey, accessErr, "project_uid", projectUID, "attempts", scrubMaxRetries)
			}
			return
		}

		slog.DebugContext(ctx, "project_subscriber: retrying scrub side effects after reload",
			"attempt", attempt+1, "project_uid", projectUID)
	}
}

// ctxWithServiceAuth returns ctx with a service-identity bearer when no inbound JWT is present.
// NATS queue subscribers have no HTTP middleware, but indexer V2 transactions require an
// authorization header in the message envelope.
func ctxWithServiceAuth(ctx context.Context) context.Context {
	if _, ok := ctx.Value(constants.AuthorizationContextID).(string); ok {
		return ctx
	}
	return context.WithValue(ctx, constants.AuthorizationContextID, serviceAuthBearer)
}

// buildProjectURL constructs the deep-link URL for a project's overview page.
func buildProjectURL(baseURL, slug string) string {
	base := strings.TrimRight(baseURL, "/") + "/project/overview"
	if slug != "" {
		return base + "?project=" + url.QueryEscape(slug)
	}
	return base
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
