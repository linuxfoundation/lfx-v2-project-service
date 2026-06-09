// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"slices"
	"time"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// InviteAcceptedService handles lfx.invite-service.invite_accepted events.
// It scans all org settings for a pending entry matching the accepted invite
// and promotes it to an accepted writer/auditor, triggering FGA + indexer republish.
type InviteAcceptedService struct {
	settingsReader    port.B2BOrgSettingsReader
	orgSettingsWriter OrgSettingsWriter
}

// InviteAcceptedServiceOption configures an InviteAcceptedService.
type InviteAcceptedServiceOption func(*InviteAcceptedService)

// WithInviteAcceptedSettingsReader sets the settingsReader.
func WithInviteAcceptedSettingsReader(r port.B2BOrgSettingsReader) InviteAcceptedServiceOption {
	return func(s *InviteAcceptedService) { s.settingsReader = r }
}

// WithInviteAcceptedOrgSettingsWriter sets the orgSettingsWriter.
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

// Handle processes an InviteServiceAcceptedEvent. It scans all org settings for a
// pending entry matching ev.UID (or fallback ev.Recipient.Email + no username) and
// promotes it to accepted. Errors per org are logged but not returned — the handler
// is best-effort because the upstream NATS subscription uses core QueueSubscribe with
// no ACK/NAK.
func (s *InviteAcceptedService) Handle(ctx context.Context, ev inviteapi.InviteServiceAcceptedEvent) error {
	if s.settingsReader == nil || s.orgSettingsWriter == nil {
		slog.ErrorContext(ctx, "InviteAcceptedService not fully initialized",
			"settingsReader", s.settingsReader != nil,
			"orgSettingsWriter", s.orgSettingsWriter != nil,
		)
		return nil
	}

	// Drop malformed events that can't be matched.
	if ev.UID == "" && ev.Recipient.Email == "" {
		slog.WarnContext(ctx, "invite_accepted event missing both UID and Recipient.Email — dropping")
		return nil
	}
	if ev.AcceptedBy == "" {
		slog.WarnContext(ctx, "invite_accepted event missing AcceptedBy — dropping", "invite_uid", ev.UID)
		return nil
	}

	// Fast path: look up the owning org directly via the secondary index (O(1)).
	// Falls through to the full scan on index miss (legacy entries written before
	// the index was introduced) or when ev.UID is empty (email-only events).
	if ev.UID != "" {
		orgUID, err := s.settingsReader.LookupInviteOrgUID(ctx, ev.UID)
		if err == nil && orgUID != "" {
			s.tryAcceptInviteInOrg(ctx, orgUID, ev)
			return nil
		}
		// Index miss or transient error → fall through to scan so the event is not dropped.
		slog.DebugContext(ctx, "invite_accepted: index miss, falling back to full scan",
			"invite_uid", ev.UID, "index_err", err)
	}

	orgUIDs, err := s.settingsReader.ListSettingsOrgUIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "invite_accepted: failed to list org settings UIDs", "error", err)
		return nil
	}

	for _, orgUID := range orgUIDs {
		s.tryAcceptInviteInOrg(ctx, orgUID, ev)
	}
	return nil
}

// tryAcceptInviteInOrg attempts to find and promote a matching pending invite in one org.
// Uses an optimistic-CAS retry loop (up to 3 attempts) to handle concurrent settings writes.
// Errors are logged but not returned.
func (s *InviteAcceptedService) tryAcceptInviteInOrg(ctx context.Context, orgUID string, ev inviteapi.InviteServiceAcceptedEvent) {
	const maxRetries = 3

	for retry := 0; retry < maxRetries; retry++ {
		settings, _, err := s.settingsReader.GetSettings(ctx, orgUID)
		if err != nil {
			slog.WarnContext(ctx, "invite_accepted: failed to get org settings",
				"org_uid", orgUID, "error", err)
			return
		}
		if settings == nil {
			return
		}

		foundIdx, foundList, found := s.findMatchingEntry(settings, ev)
		if !found {
			return
		}

		now := time.Now().UTC()

		// Patch the matched entry; leave all other fields untouched.
		if foundList == model.B2BOrgRoleWriter {
			writers := slices.Clone(settings.Writers)
			writers[foundIdx].Username = ev.AcceptedBy
			writers[foundIdx].InviteStatus = model.InviteStatusAccepted
			writers[foundIdx].AcceptedAt = &now
			writers[foundIdx].InviteUUID = ""
			writers[foundIdx].UpdatedAt = now
			_, err = s.orgSettingsWriter.Update(ctx, B2BOrgSettingsUpdate{
				OrgUID:  orgUID,
				Writers: writers,
				// Auditors nil = keep existing; only writers changed.
			})
		} else {
			auditors := slices.Clone(settings.Auditors)
			auditors[foundIdx].Username = ev.AcceptedBy
			auditors[foundIdx].InviteStatus = model.InviteStatusAccepted
			auditors[foundIdx].AcceptedAt = &now
			auditors[foundIdx].InviteUUID = ""
			auditors[foundIdx].UpdatedAt = now
			_, err = s.orgSettingsWriter.Update(ctx, B2BOrgSettingsUpdate{
				OrgUID:   orgUID,
				Auditors: auditors,
				// Writers nil = keep existing; only auditors changed.
			})
		}

		if err == nil {
			return
		}
		if pkgerrors.IsConflict(err) {
			if retry < maxRetries-1 {
				slog.DebugContext(ctx, "invite_accepted: revision conflict, retrying",
					"org_uid", orgUID, "invite_uid", ev.UID, "attempt", retry+1)
				continue
			}
			slog.WarnContext(ctx, "invite_accepted: revision conflict after 3 retries",
				"org_uid", orgUID, "invite_uid", ev.UID)
			return
		}
		slog.WarnContext(ctx, "invite_accepted: failed to update org settings",
			"org_uid", orgUID, "invite_uid", ev.UID, "error", err)
		return
	}
}

// findMatchingEntry searches settings for the pending entry to promote.
// Primary: InviteUUID == ev.UID. Fallback: pending entry with matching email and no username.
func (s *InviteAcceptedService) findMatchingEntry(settings *model.B2BOrgSettings, ev inviteapi.InviteServiceAcceptedEvent) (idx int, list string, found bool) {
	if ev.UID != "" {
		if idx, list, ok := model.FindByInviteUUID(settings, ev.UID); ok {
			return idx, list, true
		}
	}

	// Fallback: pending + email match + no username (covers legacy entries without InviteUUID).
	if ev.Recipient.Email == "" {
		return 0, "", false
	}
	for i, u := range settings.Writers {
		if u.EffectiveStatus() == model.InviteStatusPending && u.Username == "" &&
			normalizeSettingsEmail(u.Email) == normalizeSettingsEmail(ev.Recipient.Email) {
			return i, model.B2BOrgRoleWriter, true
		}
	}
	for i, u := range settings.Auditors {
		if u.EffectiveStatus() == model.InviteStatusPending && u.Username == "" &&
			normalizeSettingsEmail(u.Email) == normalizeSettingsEmail(ev.Recipient.Email) {
			return i, model.B2BOrgRoleAuditor, true
		}
	}
	return 0, "", false
}
