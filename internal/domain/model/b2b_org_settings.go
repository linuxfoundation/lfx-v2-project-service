// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"fmt"
	"time"
)

// InviteStatus represents the lifecycle state of a B2BOrgUser entry.
type InviteStatus string

const (
	// InviteStatusPending — invite sent, user has not yet accepted.
	InviteStatusPending InviteStatus = "pending"
	// InviteStatusAccepted — invite accepted; Username is set and FGA tuple is active.
	InviteStatusAccepted InviteStatus = "accepted"
	// InviteStatusRevoked — access revoked; entry retained for audit trail.
	InviteStatusRevoked InviteStatus = "revoked"
	// InviteStatusExpired — invite expired without acceptance; retained for audit trail.
	InviteStatusExpired InviteStatus = "expired"
)

// B2BOrgRole is the relation a B2BOrgUser entry grants (the InvitedAs value).
// A type alias (= string) is used so callers can assign literals without casting:
// InvitedAs is a plain string field and all comparison sites use untyped constants.
type B2BOrgRole = string

const (
	// B2BOrgRoleWriter is the relation for org administrators.
	B2BOrgRoleWriter B2BOrgRole = "writer"
	// B2BOrgRoleAuditor is the relation for read-only principals.
	B2BOrgRoleAuditor B2BOrgRole = "auditor"
)

// B2BOrgUser is a member of a b2b_org settings list (writers or auditors).
// Invite fields extend the base principal to support pre-LFID invitations.
//
// FGA tuple is emitted only when InviteStatus == InviteStatusAccepted AND
// Username is non-empty. Pending/revoked/expired entries are persisted for
// audit trail but produce no FGA tuple.
type B2BOrgUser struct {
	// Avatar is the user's avatar URL, if known.
	Avatar string `json:"avatar,omitempty"`
	// Email is the user's email address. Required; identifies the user before
	// they accept the invite and their LFID username is known.
	Email string `json:"email"`
	// Name is the user's display name.
	Name string `json:"name,omitempty"`
	// Username is the LFID username. Set once the invite is accepted.
	// Absent for pending invites.
	Username string `json:"username,omitempty"`

	// InviteUUID is the opaque token sent to the invitee. Cleared on acceptance.
	InviteUUID string `json:"invite_uuid,omitempty"`
	// InvitedAs is the relation being granted: "writer" or "auditor".
	InvitedAs string `json:"invited_as"`
	// InviteStatus is the current lifecycle state of this entry.
	InviteStatus InviteStatus `json:"invite_status"`

	// InvitedAt is when the invite was sent.
	InvitedAt *time.Time `json:"invited_at,omitempty"`
	// InvitedBy is the username of the writer who sent this invite.
	InvitedBy string `json:"invited_by,omitempty"`
	// AcceptedAt is when the invite was accepted.
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	// RevokedAt is when access was revoked.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// B2BOrgSettings holds the access-control principals for a b2b_org. Stored in a
// dedicated NATS KV bucket (org-settings) separate from the Salesforce-backed org
// record so that FGA state changes never touch Salesforce data.
type B2BOrgSettings struct {
	// UID is the b2b_org UID this settings record belongs to.
	UID string `json:"uid"`
	// Writers holds the org's administrator principals.
	// nil in PUT payload = preserve existing; explicit empty slice = clear all.
	Writers []B2BOrgUser `json:"writers"`
	// Auditors holds read-only principals.
	// nil in PUT payload = preserve existing; explicit empty slice = clear all.
	Auditors []B2BOrgUser `json:"auditors"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ActiveWriterUsernames returns the LFID usernames of writers whose invite has
// been accepted. Entries without a username (pending invites) are silently
// skipped — only accepted entries with a known username produce FGA tuples.
func (s *B2BOrgSettings) ActiveWriterUsernames() []string {
	if s == nil {
		return nil
	}
	return activeUsernames(s.Writers)
}

// ActiveAuditorUsernames returns the LFID usernames of auditors whose invite
// has been accepted. Pending/revoked/expired entries are skipped.
func (s *B2BOrgSettings) ActiveAuditorUsernames() []string {
	if s == nil {
		return nil
	}
	return activeUsernames(s.Auditors)
}

// FulltextTokens returns the name+email tokens to index for member-identity search.
// Includes accepted entries (Name + Email) and pending entries (Email + Name-if-present).
// Excludes revoked and expired — they carry no active access and would mislead search results.
func (s *B2BOrgSettings) FulltextTokens() []string {
	if s == nil {
		return nil
	}
	var tokens []string
	for _, users := range [][]B2BOrgUser{s.Writers, s.Auditors} {
		for _, u := range users {
			switch u.EffectiveStatus() {
			case InviteStatusAccepted:
				if u.Name != "" {
					tokens = append(tokens, u.Name)
				}
				if u.Email != "" {
					tokens = append(tokens, u.Email)
				}
			case InviteStatusPending:
				if u.Email != "" {
					tokens = append(tokens, u.Email)
				}
				if u.Name != "" {
					tokens = append(tokens, u.Name)
				}
			}
		}
	}
	return tokens
}

// Tag prefixes for per-user tags emitted by Tags().
const (
	// TagPrefixWriter is emitted once per accepted writer with a known LFID username.
	// Inverse query: tags=writer:<username>
	TagPrefixWriter = "writer:"
	// TagPrefixAuditor is emitted once per accepted auditor with a known LFID username.
	// Inverse query: tags=auditor:<username>
	TagPrefixAuditor = "auditor:"
	// TagPrefixMember covers both writers and auditors; use for role-agnostic
	// "which orgs does user X belong to?" queries: tags=member:<username>
	TagPrefixMember = "member:"
)

// Tags returns the discrete tag flags for the settings indexer doc.
// Includes the org UID (bare and prefixed) so the settings doc is
// discoverable by the same b2b_org_uid:<uid> tag as the org doc itself.
// has_writers: ≥1 accepted writer; has_auditors: ≥1 accepted auditor;
// has_pending_invites: ≥1 pending entry across writers or auditors.
// Revoked and expired entries do not trigger any flag.
func (s *B2BOrgSettings) Tags() []string {
	if s == nil {
		return nil
	}
	var tags []string
	if s.UID != "" {
		tags = append(tags, s.UID)
		tags = append(tags, fmt.Sprintf("b2b_org_uid:%s", s.UID))
	}
	var hasPending bool
	emittedMemberTags := map[string]struct{}{}
	hasWriters := false
	for _, u := range s.Writers {
		switch u.EffectiveStatus() {
		case InviteStatusAccepted:
			if !hasWriters {
				tags = append(tags, "has_writers")
				hasWriters = true
			}
			if u.Username != "" {
				tags = append(tags, TagPrefixWriter+u.Username)
				if _, seen := emittedMemberTags[u.Username]; !seen {
					tags = append(tags, TagPrefixMember+u.Username)
					emittedMemberTags[u.Username] = struct{}{}
				}
			}
		case InviteStatusPending:
			hasPending = true
		}
	}
	hasAuditors := false
	for _, u := range s.Auditors {
		switch u.EffectiveStatus() {
		case InviteStatusAccepted:
			if !hasAuditors {
				tags = append(tags, "has_auditors")
				hasAuditors = true
			}
			if u.Username != "" {
				tags = append(tags, TagPrefixAuditor+u.Username)
				if _, seen := emittedMemberTags[u.Username]; !seen {
					tags = append(tags, TagPrefixMember+u.Username)
					emittedMemberTags[u.Username] = struct{}{}
				}
			}
		case InviteStatusPending:
			hasPending = true
		}
	}
	if hasPending {
		tags = append(tags, "has_pending_invites")
	}
	return tags
}

// EffectiveStatus returns the entry's explicit status, or derives it from
// Username when the field is absent (legacy/admin backfill records that
// bypassed the invite flow but were written before InviteStatus was tracked).
func (u B2BOrgUser) EffectiveStatus() InviteStatus {
	if u.InviteStatus != "" {
		return u.InviteStatus
	}
	if u.Username != "" {
		return InviteStatusAccepted
	}
	return InviteStatusPending
}

// activeUsernames filters a slice to accepted entries with a non-empty username.
func activeUsernames(users []B2BOrgUser) []string {
	var out []string
	for _, u := range users {
		if u.EffectiveStatus() == InviteStatusAccepted && u.Username != "" {
			out = append(out, u.Username)
		}
	}
	return out
}
