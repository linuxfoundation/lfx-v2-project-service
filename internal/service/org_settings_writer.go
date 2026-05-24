// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
)

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

// Update applies a B2BOrgSettingsUpdate and publishes an FGA sync message.
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

	now := time.Now().UTC()
	updated := &model.B2BOrgSettings{
		UID:       in.OrgUID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if existing != nil {
		updated.CreatedAt = existing.CreatedAt
		updated.Writers = existing.Writers
		updated.Auditors = existing.Auditors
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

	o.publishFGA(ctx, in, updated)

	return updated, nil
}

func (o *orgSettingsWriterOrchestrator) publishFGA(ctx context.Context, in B2BOrgSettingsUpdate, settings *model.B2BOrgSettings) {
	if o.b2bOrgReader == nil || o.publisher == nil {
		return
	}
	org, err := o.b2bOrgReader.GetB2BOrg(ctx, in.OrgUID)
	if err != nil {
		slog.WarnContext(ctx, "could not fetch org for settings FGA publish — skipping",
			"uid", in.OrgUID, "error", err,
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
