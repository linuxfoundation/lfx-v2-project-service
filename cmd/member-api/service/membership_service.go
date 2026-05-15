// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/etag"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"

	"goa.design/goa/v3/security"
)

// membershipServicesrvc implements the generated membershipservice.Service interface.
type membershipServicesrvc struct {
	memberReaderOrchestrator usecaseSvc.MemberReader
	storage                  port.MemberReader
	auth                     domain.Authenticator
	keyContactWriter         port.KeyContactWriter
	b2bOrgReader             port.B2BOrgReader
	b2bOrgWriter             port.B2BOrgWriter
	memberPublisher          port.MemberPublisher
	globalOrgAdminTeamUID    string
}

// JWTAuth implements the authorization logic for service "membership-service".
func (s *membershipServicesrvc) JWTAuth(ctx context.Context, token string, _ *security.JWTScheme) (context.Context, error) {
	principal, err := s.auth.ParsePrincipal(ctx, token, slog.Default())
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}

// ── Health probes ─────────────────────────────────────────────────────────────

// Readyz checks if the service is ready to take inbound requests.
func (s *membershipServicesrvc) Readyz(ctx context.Context) ([]byte, error) {
	if err := s.storage.IsReady(ctx); err != nil {
		slog.ErrorContext(ctx, "service not ready", "error", err)
		return nil, err
	}
	return []byte("OK\n"), nil
}

// Livez checks if the service is alive.
func (s *membershipServicesrvc) Livez(_ context.Context) ([]byte, error) {
	return []byte("OK\n"), nil
}

// DebugVars returns the expvar debug variables as a JSON object.
func (s *membershipServicesrvc) DebugVars(_ context.Context) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	first := true
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		key, _ := json.Marshal(kv.Key)
		fmt.Fprintf(&buf, "%s: %s", key, kv.Value.String())
	})
	buf.WriteString("\n}\n")
	return buf.Bytes(), nil
}

// ── B2B Organizations ─────────────────────────────────────────────────────────

// GetB2bOrg retrieves a single B2B organization by UID.
func (s *membershipServicesrvc) GetB2bOrg(ctx context.Context, p *membershipservice.GetB2bOrgPayload) (*membershipservice.GetB2bOrgResult, error) {
	org, err := s.b2bOrgReader.GetB2BOrg(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	etagVal, etagErr := etag.LFXEtag(org)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", p.UID, "error", etagErr)
	}

	lastMod := org.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	result := &membershipservice.GetB2bOrgResult{
		B2bOrg:       b2bOrgToResponse(org),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// CreateB2bOrg creates a new B2B organization record from an existing Salesforce Account.
func (s *membershipServicesrvc) CreateB2bOrg(ctx context.Context, p *membershipservice.CreateB2bOrgPayload) (*membershipservice.CreateB2bOrgResult, error) {
	var createInput model.B2BOrgInput
	if p.ParentSfid != nil {
		if !strings.HasPrefix(*p.ParentSfid, "001") {
			return nil, wrapError(ctx, pkgerrors.NewValidation(
				fmt.Sprintf("parent_sfid %q is not a Salesforce Account ID (must start with 001)", *p.ParentSfid)))
		}
		parentUID, convErr := sfuuid.ToUUID(*p.ParentSfid)
		if convErr != nil {
			return nil, wrapError(ctx, pkgerrors.NewValidation(
				fmt.Sprintf("invalid parent_sfid %q: %v", *p.ParentSfid, convErr)))
		}
		createInput.ParentUID = &parentUID
	}
	org, err := s.b2bOrgWriter.CreateB2BOrg(ctx, p.Sfid, createInput)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	s.publishB2BOrgEvents(ctx, org, indexerConstants.ActionCreated)

	etagVal, etagErr := etag.LFXEtag(org)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", org.UID, "error", etagErr)
	}

	lastMod := org.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	result := &membershipservice.CreateB2bOrgResult{
		B2bOrg:       b2bOrgToResponse(org),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// UpdateB2bOrg updates a B2B organization.
//
// ETag validation is performed here rather than forwarded to Salesforce because
// the SF sObject PATCH endpoint does not support the If-Match header (returns
// BAD_HEADER 400). We fetch the current record, validate the caller's If-Match
// against our computed ETag, then pass If-Unmodified-Since (SF LastModifiedDate)
// to DoConditionalWrite for SF-side concurrency protection.
func (s *membershipServicesrvc) UpdateB2bOrg(ctx context.Context, p *membershipservice.UpdateB2bOrgPayload) (*membershipservice.UpdateB2bOrgResult, error) {
	input := payloadToB2BOrgInput(p)

	// Validate If-Match against the current cached ETag before touching SF.
	// Salesforce PATCH rejects If-Match (BAD_HEADER), so ETag validation is done
	// here; the caller's If-Match never reaches the infrastructure layer.
	if p.IfMatch != nil && *p.IfMatch != "" {
		current, err := s.b2bOrgReader.GetB2BOrg(ctx, p.UID)
		if err != nil {
			return nil, wrapError(ctx, err)
		}
		currentETag, err := etag.LFXEtag(current)
		if err != nil {
			return nil, wrapError(ctx, pkgerrors.NewUnexpected("failed to compute etag", err))
		}
		if currentETag != *p.IfMatch {
			return nil, wrapError(ctx, pkgerrors.NewPreconditionFailed(
				fmt.Sprintf("b2b org %s has been modified since last read (stale If-Match)", p.UID)))
		}
		// Translate to If-Unmodified-Since (SF LastModifiedDate) for the SF PATCH.
		input.IfUnmodifiedSince = current.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	}

	org, err := s.b2bOrgWriter.UpdateB2BOrg(ctx, p.UID, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	s.publishB2BOrgEvents(ctx, org, indexerConstants.ActionUpdated)

	etagVal, etagErr := etag.LFXEtag(org)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", p.UID, "error", etagErr)
	}

	lastMod := org.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	result := &membershipservice.UpdateB2bOrgResult{
		B2bOrg:       b2bOrgToResponse(org),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// publishB2BOrgEvents fans out an indexer message and an FGA update-access
// message for the given org. Publish failures on the write path are swallowed
// and logged with publish_failed_for_backfill_repair=true — the
// /admin/reindex endpoint recovers missed records.
func (s *membershipServicesrvc) publishB2BOrgEvents(ctx context.Context, org *model.B2BOrg, action indexerConstants.MessageAction) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           org.Tags(),
		IndexingConfig: buildB2BOrgIndexingConfig(org),
	}
	builtMsg, err := indexMsg.Build(ctx, org)
	if err != nil {
		slog.WarnContext(ctx, "failed to build b2b org indexer message",
			"uid", org.UID,
			"error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}

	fgaMsg := buildB2BOrgFGAMessage(org, s.globalOrgAdminTeamUID)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.memberPublisher.Indexer(gCtx, constants.IndexB2BOrgSubject, builtMsg, false)
	})
	g.Go(func() error {
		return s.memberPublisher.Access(gCtx, constants.FGASyncUpdateAccessSubject, fgaMsg, false)
	})

	if pubErr := g.Wait(); pubErr != nil {
		slog.WarnContext(ctx, "b2b org event publish failed",
			"uid", org.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// ── Project Memberships (Stubs) ───────────────────────────────────────────────

// GetProjectMembership retrieves a single membership by UID.
func (s *membershipServicesrvc) GetProjectMembership(ctx context.Context, p *membershipservice.GetProjectMembershipPayload) (*membershipservice.GetProjectMembershipResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("get-project-membership not implemented"))
}

// ── Key Contacts (Stubs) ──────────────────────────────────────────────────────

// GetKeyContact retrieves a single key contact by UID.
func (s *membershipServicesrvc) GetKeyContact(ctx context.Context, p *membershipservice.GetKeyContactPayload) (*membershipservice.GetKeyContactResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("get-key-contact not implemented"))
}

// CreateKeyContact creates a new key contact.
func (s *membershipServicesrvc) CreateKeyContact(ctx context.Context, p *membershipservice.CreateKeyContactPayload) (*membershipservice.CreateKeyContactResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("create-key-contact not implemented"))
}

// UpdateKeyContact updates a key contact.
func (s *membershipServicesrvc) UpdateKeyContact(ctx context.Context, p *membershipservice.UpdateKeyContactPayload) (*membershipservice.UpdateKeyContactResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("update-key-contact not implemented"))
}

// DeleteKeyContact deletes a key contact.
func (s *membershipServicesrvc) DeleteKeyContact(ctx context.Context, p *membershipservice.DeleteKeyContactPayload) error {
	return wrapError(ctx, pkgerrors.NewNotImplemented("delete-key-contact not implemented"))
}

// ── Admin (Stubs) ─────────────────────────────────────────────────────────────

// AdminReindex triggers a reindex of cached entities.
func (s *membershipServicesrvc) AdminReindex(ctx context.Context, p *membershipservice.AdminReindexPayload) (*membershipservice.AdminReindexResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("admin-reindex not implemented"))
}

// ── Response converters ───────────────────────────────────────────────────────

// b2bOrgToResponse converts a domain B2BOrg to the generated response type.
func b2bOrgToResponse(org *model.B2BOrg) *membershipservice.B2bOrgResponse {
	resp := &membershipservice.B2bOrgResponse{
		UID:  &org.UID,
		Name: &org.Name,
	}
	if org.Description != "" {
		resp.Description = &org.Description
	}
	if org.Phone != "" {
		resp.Phone = &org.Phone
	}
	if org.Website != "" {
		resp.Website = &org.Website
	}
	if org.PrimaryDomain != "" {
		resp.PrimaryDomain = &org.PrimaryDomain
	}
	if len(org.DomainAliases) > 0 {
		resp.DomainAliases = org.DomainAliases
	}
	if org.LogoURL != "" {
		resp.LogoURL = &org.LogoURL
	}
	if org.Industry != "" {
		resp.Industry = &org.Industry
	}
	if org.Sector != "" {
		resp.Sector = &org.Sector
	}
	if org.CrunchBaseURL != nil {
		resp.CrunchBaseURL = org.CrunchBaseURL
	}
	if org.NumberOfEmployees != nil {
		n := int(*org.NumberOfEmployees)
		resp.NumberOfEmployees = &n
	}
	if org.Status != "" {
		resp.Status = &org.Status
	}
	resp.IsMember = &org.IsMember
	if org.Slug != "" {
		resp.Slug = &org.Slug
	}
	if org.ParentUID != "" {
		resp.ParentUID = &org.ParentUID
	}
	createdAt := org.CreatedAt.UTC().Format(time.RFC3339)
	resp.CreatedAt = &createdAt
	updatedAt := org.UpdatedAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = &updatedAt
	return resp
}

// payloadToB2BOrgInput maps an UpdateB2bOrgPayload to a model.B2BOrgInput.
func payloadToB2BOrgInput(p *membershipservice.UpdateB2bOrgPayload) model.B2BOrgInput {
	input := model.B2BOrgInput{}
	if p.IfUnmodifiedSince != nil {
		input.IfUnmodifiedSince = *p.IfUnmodifiedSince
	}
	if p.Name != nil {
		input.Name = *p.Name
	}
	if p.Description != nil {
		input.Description = *p.Description
	}
	if p.Phone != nil {
		input.Phone = *p.Phone
	}
	if p.Website != nil {
		input.Website = *p.Website
	}
	if p.PrimaryDomain != nil {
		input.PrimaryDomain = *p.PrimaryDomain
	}
	if p.LogoURL != nil {
		input.LogoURL = *p.LogoURL
	}
	if p.Industry != nil {
		input.Industry = *p.Industry
	}
	if p.Sector != nil {
		input.Sector = *p.Sector
	}
	if p.CrunchBaseURL != nil {
		input.CrunchBaseURL = p.CrunchBaseURL
	}
	if p.NumberOfEmployees != nil {
		n := int64(*p.NumberOfEmployees)
		input.NumberOfEmployees = &n
	}
	return input
}

// ── Constructor ───────────────────────────────────────────────────────────────

// NewMembershipService returns the membership-service implementation with
// injected dependencies.
func NewMembershipService(
	readMemberUseCase usecaseSvc.MemberReader,
	storage port.MemberReader,
	authenticator domain.Authenticator,
	keyContactWriter port.KeyContactWriter,
	b2bOrgReader port.B2BOrgReader,
	b2bOrgWriter port.B2BOrgWriter,
	memberPublisher port.MemberPublisher,
	globalOrgAdminTeamUID string,
) membershipservice.Service {
	return &membershipServicesrvc{
		memberReaderOrchestrator: readMemberUseCase,
		storage:                  storage,
		auth:                     authenticator,
		keyContactWriter:         keyContactWriter,
		b2bOrgReader:             b2bOrgReader,
		b2bOrgWriter:             b2bOrgWriter,
		memberPublisher:          memberPublisher,
		globalOrgAdminTeamUID:    globalOrgAdminTeamUID,
	}
}
