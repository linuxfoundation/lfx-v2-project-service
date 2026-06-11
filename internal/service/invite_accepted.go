// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/redaction"
)

// orgSettingsUpdater is a narrow consumer-side interface — InviteAcceptedService only
// needs Update, not the full OrgSettingsWriter surface.
type orgSettingsUpdater interface {
	Update(ctx context.Context, in B2BOrgSettingsUpdate) (*model.B2BOrgSettings, error)
}

// InviteAcceptedService handles lfx.invite-service.invite_accepted events.
// It filters to b2b_org events, then scans all org settings for pending entries
// matching the accepted invite's email and promotes them to accepted — triggering
// FGA + indexer republish. This mirrors the committee/project scan pattern.
type InviteAcceptedService struct {
	settingsReader    port.B2BOrgSettingsReader
	orgSettingsWriter orgSettingsUpdater
}

// InviteAcceptedServiceOption configures an InviteAcceptedService.
type InviteAcceptedServiceOption func(*InviteAcceptedService)

// WithInviteAcceptedSettingsReader sets the settingsReader.
func WithInviteAcceptedSettingsReader(r port.B2BOrgSettingsReader) InviteAcceptedServiceOption {
	return func(s *InviteAcceptedService) { s.settingsReader = r }
}

// WithInviteAcceptedOrgSettingsWriter sets the orgSettingsWriter. Any value that
// satisfies OrgSettingsWriter (the full use-case interface) also satisfies the
// narrower orgSettingsUpdater — no adapter needed.
func WithInviteAcceptedOrgSettingsWriter(w OrgSettingsWriter) InviteAcceptedServiceOption {
	return func(s *InviteAcceptedService) { s.orgSettingsWriter = w }
}

// NewInviteAcceptedService creates a new InviteAcceptedService.
func NewInviteAcceptedService(opts ...InviteAcceptedServiceOption) *InviteAcceptedService {
	s := &InviteAcceptedService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handle processes an InviteServiceAcceptedEvent.
//
// Early exits (no KV access):
//   - ev.Recipient.Email or ev.AcceptedBy is empty — malformed, drop + warn.
//   - ev.Resource.Type != "b2b_org" — belongs to committee/project, not us; drop silently.
//
// Otherwise, scans all org settings for pending entries whose email matches
// ev.Recipient.Email and promotes them. Errors per org are logged but not returned —
// the handler is best-effort because the upstream NATS subscription uses core
// QueueSubscribe with no ACK/NAK.
func (s *InviteAcceptedService) Handle(ctx context.Context, ev inviteapi.InviteServiceAcceptedEvent) error {
	if s.settingsReader == nil || s.orgSettingsWriter == nil {
		slog.ErrorContext(ctx, "InviteAcceptedService not fully initialized",
			"settingsReader", s.settingsReader != nil,
			"orgSettingsWriter", s.orgSettingsWriter != nil,
		)
		return nil
	}

	// Validate required fields first so we never scan on a malformed event.
	if strings.TrimSpace(ev.Recipient.Email) == "" || ev.AcceptedBy == "" {
		slog.WarnContext(ctx, "invite_accepted event missing required fields — dropping",
			"accepted_by", redaction.Redact(ev.AcceptedBy),
			"recipient_email", redaction.RedactEmail(ev.Recipient.Email),
		)
		return nil
	}

	// Filter to org events only. Member-service receives every invite_accepted event
	// (no resource-type filter on the subscription), so committee and project
	// acceptances arrive here too. Drop them with zero KV access.
	if ev.Resource.Type != "b2b_org" {
		slog.DebugContext(ctx, "invite_accepted: skipping non-org resource type",
			"resource_type", ev.Resource.Type,
		)
		return nil
	}

	orgUIDs, err := s.settingsReader.ListSettingsOrgUIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "invite_accepted: failed to list org settings UIDs", "error", err)
		return nil
	}

	normalizedEmail := normalizeSettingsEmail(ev.Recipient.Email)
	for _, orgUID := range orgUIDs {
		s.tryAcceptInviteInOrg(ctx, orgUID, normalizedEmail, ev)
	}
	return nil
}

// tryAcceptInviteInOrg attempts to find and promote all pending entries matching
// normalizedEmail in one org. Uses a list-authoritative + role tie-break strategy:
//   - Entries in exactly one list → promote all of them.
//   - Entries in both lists → ev.Role selects: Manage→writers, View→auditors.
//     Unknown/empty role → skip + warn (no over-grant).
//
// Uses an optimistic-CAS retry loop (up to 3 attempts) to handle concurrent writes.
// Errors are logged but not returned.
func (s *InviteAcceptedService) tryAcceptInviteInOrg(ctx context.Context, orgUID, normalizedEmail string, ev inviteapi.InviteServiceAcceptedEvent) {
	const maxRetries = 3

	for retry := range maxRetries {
		settings, _, err := s.settingsReader.GetSettings(ctx, orgUID)
		if err != nil {
			slog.WarnContext(ctx, "invite_accepted: failed to get org settings",
				"org_uid", orgUID, "error", err)
			return
		}
		if settings == nil {
			return
		}

		// Snapshot the ETag now so Update's IfMatch check catches any concurrent
		// write that changes the lists between this read and Update's own read.
		// On mismatch Update returns PreconditionFailed, which the retry loop handles.
		ifMatch, _ := etag.LFXEtag(settings)

		writerIdxs := pendingEmailIndices(settings.Writers, normalizedEmail)
		auditorIdxs := pendingEmailIndices(settings.Auditors, normalizedEmail)

		if len(writerIdxs) == 0 && len(auditorIdxs) == 0 {
			return // no match in this org
		}

		// Determine which list(s) to promote.
		var promoteWriters, promoteAuditors bool
		switch {
		case len(writerIdxs) > 0 && len(auditorIdxs) == 0:
			promoteWriters = true
		case len(writerIdxs) == 0 && len(auditorIdxs) > 0:
			promoteAuditors = true
		default:
			// Email appears in both lists — use ev.Role as tie-breaker.
			switch ev.Role {
			case string(inviteapi.InviteRoleManage):
				promoteWriters = true
			case string(inviteapi.InviteRoleView):
				promoteAuditors = true
			default:
				slog.WarnContext(ctx, "invite_accepted: email in both writers and auditors with unresolvable role — skipping org",
					"org_uid", orgUID,
					"email", redaction.RedactEmail(ev.Recipient.Email),
					"role", ev.Role,
				)
				return
			}
		}

		now := time.Now().UTC()
		update := B2BOrgSettingsUpdate{OrgUID: orgUID, IfMatch: ifMatch}

		if promoteWriters {
			writers := slices.Clone(settings.Writers)
			for _, i := range writerIdxs {
				writers[i].Username = ev.AcceptedBy
				writers[i].InviteStatus = model.InviteStatusAccepted
				writers[i].AcceptedAt = &now
				writers[i].InviteUUID = ""
				writers[i].UpdatedAt = now
			}
			update.Writers = writers
		}
		if promoteAuditors {
			auditors := slices.Clone(settings.Auditors)
			for _, i := range auditorIdxs {
				auditors[i].Username = ev.AcceptedBy
				auditors[i].InviteStatus = model.InviteStatusAccepted
				auditors[i].AcceptedAt = &now
				auditors[i].InviteUUID = ""
				auditors[i].UpdatedAt = now
			}
			update.Auditors = auditors
		}

		_, err = s.orgSettingsWriter.Update(ctx, update)
		if err == nil {
			return
		}
		if pkgerrors.IsConflict(err) || pkgerrors.IsPreconditionFailed(err) {
			if retry < maxRetries-1 {
				slog.DebugContext(ctx, "invite_accepted: revision conflict, retrying",
					"org_uid", orgUID, "attempt", retry+1)
				continue
			}
			slog.WarnContext(ctx, "invite_accepted: revision conflict after 3 retries",
				"org_uid", orgUID)
			return
		}
		slog.WarnContext(ctx, "invite_accepted: failed to update org settings",
			"org_uid", orgUID, "error", err)
		return
	}
}

// pendingEmailIndices returns the indices of pending entries (no username set)
// in users whose normalised email matches normalizedEmail.
func pendingEmailIndices(users []model.B2BOrgUser, normalizedEmail string) []int {
	var out []int
	for i, u := range users {
		if u.EffectiveStatus() == model.InviteStatusPending &&
			u.Username == "" &&
			normalizeSettingsEmail(u.Email) == normalizedEmail {
			out = append(out, i)
		}
	}
	return out
}
