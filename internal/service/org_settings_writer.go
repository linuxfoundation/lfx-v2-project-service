// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
)

// maxPrincipals bounds the writers/auditors list length to prevent unbounded NATS KV
// value growth. Largest prod orgs carry ~300 principals; 700 gives comfortable headroom
// while remaining a practical safety bound against runaway callers.
const maxPrincipals = 700

// B2BOrgSettingsUpdate carries the validated parameters for a settings update.
// Writers/Auditors nil = keep existing; explicit empty slice = clear all.
// IfMatch == "" means first-write-wins (no ETag check).
type B2BOrgSettingsUpdate struct {
	OrgUID   string
	Writers  []model.B2BOrgUser // nil = keep, [] = clear all
	Auditors []model.B2BOrgUser // nil = keep, [] = clear all
	IfMatch  string
}

// OrgSettingsWriter orchestrates the UpdateB2bOrgSettings use case.
type OrgSettingsWriter interface {
	Update(ctx context.Context, in B2BOrgSettingsUpdate) (*model.B2BOrgSettings, error)
	// AddPrincipal adds (invites) one principal, preserving every existing member.
	AddPrincipal(ctx context.Context, in B2BOrgSettingsAddPrincipal) (*model.B2BOrgSettings, error)
	// ChangePrincipalRole moves one principal between writers/auditors, preserving
	// its username and invite lifecycle so an accepted grant stays accepted.
	ChangePrincipalRole(ctx context.Context, in B2BOrgSettingsChangeRole) (*model.B2BOrgSettings, error)
	// RemovePrincipal removes one principal (revoke accepted grant or cancel pending invite).
	RemovePrincipal(ctx context.Context, in B2BOrgSettingsRemovePrincipal) (*model.B2BOrgSettings, error)
}

// B2BOrgSettingsAddPrincipal carries the validated parameters for a per-principal add.
type B2BOrgSettingsAddPrincipal struct {
	OrgUID    string
	Email     string
	InvitedAs string // "writer" or "auditor"
	Name      string
}

// B2BOrgSettingsChangeRole carries the validated parameters for a per-principal role change.
type B2BOrgSettingsChangeRole struct {
	OrgUID    string
	Email     string
	InvitedAs string // target relation: "writer" or "auditor"
	IfMatch   string
}

// B2BOrgSettingsRemovePrincipal carries the validated parameters for a per-principal remove.
type B2BOrgSettingsRemovePrincipal struct {
	OrgUID  string
	Email   string
	IfMatch string
}

type orgSettingsWriterOrchestrator struct {
	settingsReader port.B2BOrgSettingsReader
	settingsWriter port.B2BOrgSettingsWriter
	b2bOrgReader   port.B2BOrgReader
	publisher      port.MemberPublisher
}

// OrgSettingsWriterOption configures an orgSettingsWriterOrchestrator.
type OrgSettingsWriterOption func(*orgSettingsWriterOrchestrator)

func WithOrgSettingsReader(r port.B2BOrgSettingsReader) OrgSettingsWriterOption {
	return func(o *orgSettingsWriterOrchestrator) { o.settingsReader = r }
}

func WithOrgSettingsWriter(w port.B2BOrgSettingsWriter) OrgSettingsWriterOption {
	return func(o *orgSettingsWriterOrchestrator) { o.settingsWriter = w }
}

func WithOrgSettingsB2BOrgReader(r port.B2BOrgReader) OrgSettingsWriterOption {
	return func(o *orgSettingsWriterOrchestrator) { o.b2bOrgReader = r }
}

func WithOrgSettingsPublisher(p port.MemberPublisher) OrgSettingsWriterOption {
	return func(o *orgSettingsWriterOrchestrator) { o.publisher = p }
}

// NewOrgSettingsWriter constructs an OrgSettingsWriter.
func NewOrgSettingsWriter(opts ...OrgSettingsWriterOption) OrgSettingsWriter {
	o := &orgSettingsWriterOrchestrator{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Update applies a B2BOrgSettingsUpdate and publishes FGA sync and indexer messages.
// GetB2BOrg is called once here and shared by both publish helpers to avoid a double fetch.
// FGA is published before the indexer so access tuples land before the doc is searchable.
func (o *orgSettingsWriterOrchestrator) Update(ctx context.Context, in B2BOrgSettingsUpdate) (*model.B2BOrgSettings, error) {
	existing, revision, err := o.settingsReader.GetSettings(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}

	// Optional ETag pre-check: if caller supplied If-Match, validate before writing.
	if in.IfMatch != "" {
		if existing == nil {
			return nil, pkgerrors.NewPreconditionFailed("no settings record exists to match against — omit If-Match for first write")
		}
		currentETag, etagErr := etag.LFXEtag(existing)
		if etagErr != nil {
			return nil, pkgerrors.NewUnexpected("failed to compute etag for settings", etagErr)
		}
		if currentETag != in.IfMatch {
			return nil, pkgerrors.NewPreconditionFailed("settings have been modified since your last read — refresh and retry")
		}
	}

	// Both nil means the caller omitted both fields — semantic no-op, nothing to write or publish.
	if in.Writers == nil && in.Auditors == nil {
		if existing == nil {
			return &model.B2BOrgSettings{UID: in.OrgUID}, nil
		}
		return existing, nil
	}

	// Bound slice length to prevent unbounded NATS KV value growth.
	if len(in.Writers) > maxPrincipals || len(in.Auditors) > maxPrincipals {
		return nil, pkgerrors.NewValidation(fmt.Sprintf("writers and auditors lists must not exceed %d entries each", maxPrincipals))
	}

	now := time.Now().UTC()
	updated := &model.B2BOrgSettings{
		UID:       in.OrgUID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if existing != nil {
		updated.CreatedAt = existing.CreatedAt
		updated.Writers = slices.Clone(existing.Writers)
		updated.Auditors = slices.Clone(existing.Auditors)
	}

	if in.Writers != nil {
		updated.Writers = in.Writers
	}
	if in.Auditors != nil {
		updated.Auditors = in.Auditors
	}

	if err := o.settingsWriter.UpdateSettings(ctx, updated, revision); err != nil {
		return nil, err
	}

	// Fetch org once; share across both publish helpers.
	action := indexerConstants.ActionUpdated
	if existing == nil {
		action = indexerConstants.ActionCreated
	}
	o.publishAll(ctx, in, updated, action)

	return updated, nil
}

// AddPrincipal adds (invites) one principal to writers/auditors. Existing members are
// preserved verbatim (full structs, incl. username/invite lifecycle). A live grant for the
// same email (accepted or pending) is a Conflict; a revoked/expired entry is replaced.
func (o *orgSettingsWriterOrchestrator) AddPrincipal(ctx context.Context, in B2BOrgSettingsAddPrincipal) (*model.B2BOrgSettings, error) {
	email := normalizeSettingsEmail(in.Email)
	if email == "" {
		return nil, pkgerrors.NewValidation("email is required")
	}
	if in.InvitedAs != "writer" && in.InvitedAs != "auditor" {
		return nil, pkgerrors.NewValidation("invited_as must be writer or auditor")
	}

	existing, revision, err := o.settingsReader.GetSettings(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	updated := cloneSettings(existing, in.OrgUID, now)

	if current, found := findPrincipalByEmail(updated, email); found {
		status := current.EffectiveStatus()
		if status != model.InviteStatusRevoked && status != model.InviteStatusExpired {
			return nil, pkgerrors.NewConflict("this person already has access or a pending invite")
		}
		// Revoked/expired audit entry — drop it before re-inviting.
		updated.Writers = removePrincipalByEmail(updated.Writers, email)
		updated.Auditors = removePrincipalByEmail(updated.Auditors, email)
	}

	entry := model.B2BOrgUser{
		Email:        email,
		Name:         strings.TrimSpace(in.Name),
		InvitedAs:    in.InvitedAs,
		InviteStatus: model.InviteStatusPending,
		InvitedAt:    &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if in.InvitedAs == "writer" {
		updated.Writers = append(updated.Writers, entry)
	} else {
		updated.Auditors = append(updated.Auditors, entry)
	}

	// Bound slice length to prevent unbounded NATS KV value growth (parity with Update).
	if len(updated.Writers) > maxPrincipals || len(updated.Auditors) > maxPrincipals {
		return nil, pkgerrors.NewValidation(fmt.Sprintf("writers and auditors lists must not exceed %d entries each", maxPrincipals))
	}

	return o.persistAndPublish(ctx, in.OrgUID, existing, updated, revision)
}

// ChangePrincipalRole moves one principal between writers and auditors, preserving
// its full struct (username, invite_status, timestamps) so an accepted grant stays accepted.
func (o *orgSettingsWriterOrchestrator) ChangePrincipalRole(ctx context.Context, in B2BOrgSettingsChangeRole) (*model.B2BOrgSettings, error) {
	email := normalizeSettingsEmail(in.Email)
	if email == "" {
		return nil, pkgerrors.NewValidation("email is required")
	}
	if in.InvitedAs != "writer" && in.InvitedAs != "auditor" {
		return nil, pkgerrors.NewValidation("invited_as must be writer or auditor")
	}

	existing, revision, err := o.settingsReader.GetSettings(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("no settings record exists for this organization")
	}
	if err := checkSettingsIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	updated := cloneSettings(existing, in.OrgUID, now)
	current, found := findPrincipalByEmail(updated, email)
	if !found {
		return nil, pkgerrors.NewNotFound("principal not found for this organization")
	}

	moved := current
	moved.InvitedAs = in.InvitedAs
	moved.UpdatedAt = now
	updated.Writers = removePrincipalByEmail(updated.Writers, email)
	updated.Auditors = removePrincipalByEmail(updated.Auditors, email)
	if in.InvitedAs == "writer" {
		updated.Writers = append(updated.Writers, moved)
	} else {
		updated.Auditors = append(updated.Auditors, moved)
	}

	if err := assertLastAdminInvariant(updated); err != nil {
		return nil, err
	}
	return o.persistAndPublish(ctx, in.OrgUID, existing, updated, revision)
}

// RemovePrincipal removes one principal (revoke accepted grant or cancel pending invite),
// leaving every other member untouched. The last accepted Admin cannot be removed.
func (o *orgSettingsWriterOrchestrator) RemovePrincipal(ctx context.Context, in B2BOrgSettingsRemovePrincipal) (*model.B2BOrgSettings, error) {
	email := normalizeSettingsEmail(in.Email)
	if email == "" {
		return nil, pkgerrors.NewValidation("email is required")
	}

	existing, revision, err := o.settingsReader.GetSettings(ctx, in.OrgUID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pkgerrors.NewNotFound("no settings record exists for this organization")
	}
	if err := checkSettingsIfMatch(existing, in.IfMatch); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	updated := cloneSettings(existing, in.OrgUID, now)
	if _, found := findPrincipalByEmail(updated, email); !found {
		return nil, pkgerrors.NewNotFound("principal not found for this organization")
	}
	updated.Writers = removePrincipalByEmail(updated.Writers, email)
	updated.Auditors = removePrincipalByEmail(updated.Auditors, email)

	if err := assertLastAdminInvariant(updated); err != nil {
		return nil, err
	}
	return o.persistAndPublish(ctx, in.OrgUID, existing, updated, revision)
}

// persistAndPublish writes the merged settings (optimistic CAS via revision) and fires the
// FGA + indexer publishes. Both relation lists are passed non-nil so the FGA full-sync runs
// for writers and auditors (adding new tuples and revoking removed ones).
func (o *orgSettingsWriterOrchestrator) persistAndPublish(
	ctx context.Context,
	orgUID string,
	existing, updated *model.B2BOrgSettings,
	revision uint64,
) (*model.B2BOrgSettings, error) {
	updated.UpdatedAt = time.Now().UTC()
	if err := o.settingsWriter.UpdateSettings(ctx, updated, revision); err != nil {
		return nil, err
	}
	action := indexerConstants.ActionUpdated
	if existing == nil {
		action = indexerConstants.ActionCreated
	}
	// Non-nil slices signal "replace this relation" so the FGA full-sync reconciles tuples.
	in := B2BOrgSettingsUpdate{
		OrgUID:   orgUID,
		Writers:  updated.Writers,
		Auditors: updated.Auditors,
	}
	o.publishAll(ctx, in, updated, action)
	return updated, nil
}

// normalizeSettingsEmail lowercases and trims an email for case-insensitive matching.
func normalizeSettingsEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// cloneSettings shallow-copies the writers/auditors slices so callers can mutate the copy
// without touching the reader's cached value. A nil source yields a fresh settings record.
func cloneSettings(s *model.B2BOrgSettings, orgUID string, now time.Time) *model.B2BOrgSettings {
	if s == nil {
		return &model.B2BOrgSettings{UID: orgUID, CreatedAt: now, UpdatedAt: now}
	}
	return &model.B2BOrgSettings{
		UID:       s.UID,
		Writers:   slices.Clone(s.Writers),
		Auditors:  slices.Clone(s.Auditors),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// findPrincipalByEmail returns a copy of the matching entry (writers checked first) and whether it was found.
func findPrincipalByEmail(s *model.B2BOrgSettings, email string) (model.B2BOrgUser, bool) {
	for _, list := range [][]model.B2BOrgUser{s.Writers, s.Auditors} {
		for _, u := range list {
			if normalizeSettingsEmail(u.Email) == email {
				return u, true
			}
		}
	}
	return model.B2BOrgUser{}, false
}

// removePrincipalByEmail returns a new slice with any entry matching email removed.
func removePrincipalByEmail(users []model.B2BOrgUser, email string) []model.B2BOrgUser {
	out := make([]model.B2BOrgUser, 0, len(users))
	for _, u := range users {
		if normalizeSettingsEmail(u.Email) == email {
			continue
		}
		out = append(out, u)
	}
	return out
}

// assertLastAdminInvariant rejects a mutation that would leave the org with zero accepted writers.
func assertLastAdminInvariant(s *model.B2BOrgSettings) error {
	accepted := 0
	for _, u := range s.Writers {
		if u.EffectiveStatus() == model.InviteStatusAccepted {
			accepted++
		}
	}
	if accepted == 0 {
		return pkgerrors.NewConflict("organization must keep at least one Admin")
	}
	return nil
}

// checkSettingsIfMatch validates the optional If-Match precondition against the current settings ETag.
func checkSettingsIfMatch(existing *model.B2BOrgSettings, ifMatch string) error {
	if ifMatch == "" {
		return nil
	}
	if existing == nil {
		return pkgerrors.NewPreconditionFailed("no settings record exists to match against — omit If-Match for first write")
	}
	currentETag, err := etag.LFXEtag(existing)
	if err != nil {
		return pkgerrors.NewUnexpected("failed to compute etag for settings", err)
	}
	if currentETag != ifMatch {
		return pkgerrors.NewPreconditionFailed("settings have been modified since your last read — refresh and retry")
	}
	return nil
}

// publishAll fetches the parent org once and drives both the FGA and indexer publishes.
// Both are fire-and-forget: errors are logged, never returned to the caller.
// FGA is published first so access tuples land before the indexer doc is searchable.
func (o *orgSettingsWriterOrchestrator) publishAll(ctx context.Context, in B2BOrgSettingsUpdate, settings *model.B2BOrgSettings, action indexerConstants.MessageAction) {
	if o.b2bOrgReader == nil || o.publisher == nil {
		return
	}
	org, err := o.b2bOrgReader.GetB2BOrg(ctx, in.OrgUID)
	if err != nil {
		slog.WarnContext(ctx, "could not fetch org for settings publish — skipping FGA and indexer",
			"uid", in.OrgUID, "error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	if org == nil {
		slog.WarnContext(ctx, "org fetch returned nil with no error — skipping FGA and indexer",
			"uid", in.OrgUID,
			"publish_failed_for_backfill_repair", true)
		return
	}

	fgaMsg := BuildB2BOrgFGAMessage(
		org,
		"",
		fgaUsernames(in.Writers, settings.ActiveWriterUsernames()),
		fgaUsernames(in.Auditors, settings.ActiveAuditorUsernames()),
		nil,
	)
	if pubErr := o.publisher.Access(ctx, constants.FGASyncUpdateAccessSubject, fgaMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "b2b org settings FGA publish failed",
			"uid", in.OrgUID, "error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}

	PublishB2BOrgSettingsIndexer(ctx, o.publisher, org, settings, action)
}

// fgaUsernames maps the nil-vs-empty distinction from the input slice through to
// the FGA sync layer. nil input = caller did not touch this relation → return nil
// so BuildB2BOrgFGAMessage excludes it from the full-sync (preserving existing
// tuples). Non-nil input = caller explicitly replaced the list → return a non-nil
// slice (possibly empty) so the full-sync runs and revokes any removed tuples.
func fgaUsernames(input []model.B2BOrgUser, active []string) []string {
	if input == nil {
		return nil
	}
	if active == nil {
		return []string{} // non-nil empty: signal "replace with nothing"
	}
	return active
}
