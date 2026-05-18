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
	projectMembershipReader  port.ProjectMembershipReader
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

	lastMod := org.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
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
	org, err := s.b2bOrgWriter.CreateB2BOrg(ctx, p.Sfid, createInput)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	s.publishB2BOrgEvents(ctx, org, indexerConstants.ActionCreated)

	etagVal, etagErr := etag.LFXEtag(org)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", org.UID, "error", etagErr)
	}

	lastMod := org.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
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

	// SF PATCH rejects If-Match (returns BAD_HEADER), so ETag validation is done
	// here; we translate to If-Unmodified-Since for SF-side concurrency protection.
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
		input.IfUnmodifiedSince = current.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	}

	if !input.HasChanges() {
		org, err := s.b2bOrgReader.GetB2BOrg(ctx, p.UID)
		if err != nil {
			return nil, wrapError(ctx, err)
		}
		etagVal, etagErr := etag.LFXEtag(org)
		if etagErr != nil {
			slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", p.UID, "error", etagErr)
		}
		lastMod := org.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
		result := &membershipservice.UpdateB2bOrgResult{
			B2bOrg:       b2bOrgToResponse(org),
			LastModified: &lastMod,
		}
		if etagVal != "" {
			result.Etag = &etagVal
		}
		return result, nil
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

	lastMod := org.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	result := &membershipservice.UpdateB2bOrgResult{
		B2bOrg:       b2bOrgToResponse(org),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// publishKeyContactEvents fans out an indexer message and an FGA access message
// for the given key contact. fgaSubject selects update-access vs delete-access;
// backfillRepair controls the publish_failed_for_backfill_repair log field
// (false for deletes — dangling FGA permissions are not auto-repairable).
func (s *membershipServicesrvc) publishKeyContactEvents(ctx context.Context, kc *model.KeyContact, action indexerConstants.MessageAction, fgaSubject string, backfillRepair bool) {
	indexMsg := &model.MemberIndexerMessage{
		Action:         action,
		Tags:           kc.Tags(),
		IndexingConfig: buildKeyContactIndexingConfig(kc),
	}
	builtMsg, err := indexMsg.Build(ctx, kc)
	if err != nil {
		slog.WarnContext(ctx, "failed to build key contact indexer message",
			"uid", kc.UID,
			"error", err,
			"publish_failed_for_backfill_repair", backfillRepair)
		return
	}

	fgaMsg := buildKeyContactFGAMessage(kc)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.memberPublisher.Indexer(gCtx, constants.IndexKeyContactSubject, builtMsg, false)
	})
	g.Go(func() error {
		return s.memberPublisher.Access(gCtx, fgaSubject, fgaMsg, false)
	})

	if pubErr := g.Wait(); pubErr != nil {
		slog.WarnContext(ctx, "key contact event publish failed",
			"uid", kc.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", backfillRepair)
	}
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

	orgAdminTeamUID := ""
	if action == indexerConstants.ActionCreated {
		orgAdminTeamUID = s.globalOrgAdminTeamUID
	}
	fgaMsg := buildB2BOrgFGAMessage(org, orgAdminTeamUID)

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

// ── Project Memberships ──────────────────────────────────────────────────────

// GetProjectMembership retrieves a single membership by UID and assembles the
// fully denormalised record from its constituent Salesforce objects.
func (s *membershipServicesrvc) GetProjectMembership(ctx context.Context, p *membershipservice.GetProjectMembershipPayload) (*membershipservice.GetProjectMembershipResult, error) {
	membership, lastMod, err := s.projectMembershipReader.AssembleProjectMembership(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	etagVal, etagErr := etag.LFXEtag(membership)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for project membership", "uid", p.UID, "error", etagErr)
	}

	lastModStr := lastMod.UTC().Format(constants.HTTPDateFormat)
	result := &membershipservice.GetProjectMembershipResult{
		ProjectMembership: projectMembershipToResponse(membership),
		LastModified:      &lastModStr,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// ── Key Contacts ─────────────────────────────────────────────────────────────

// GetKeyContact retrieves a single key contact by UID.
func (s *membershipServicesrvc) GetKeyContact(ctx context.Context, p *membershipservice.GetKeyContactPayload) (*membershipservice.GetKeyContactResult, error) {
	kc, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	etagVal, etagErr := etag.LFXEtag(kc)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for key contact", "uid", p.UID, "error", etagErr)
	}

	lastMod := kc.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	result := &membershipservice.GetKeyContactResult{
		KeyContact:   keyContactToResponse(kc),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// CreateKeyContact creates a new key contact.
func (s *membershipServicesrvc) CreateKeyContact(ctx context.Context, p *membershipservice.CreateKeyContactPayload) (*membershipservice.CreateKeyContactResult, error) {
	// Normalize and validate: lowercase email, trim names, check capacity, detect self-heal.
	existing, err := s.normalizeAndValidateCreate(ctx, p)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Self-heal: if duplicate found, return it as-is without writer call or event publish.
	// Publish is intentionally skipped — the record already exists in the indexer/FGA.
	// If a prior publish silently failed, recovery is via /admin/reindex (see message_builders.go).
	if existing != nil {
		etagVal, etagErr := etag.LFXEtag(existing)
		if etagErr != nil {
			slog.WarnContext(ctx, "failed to compute etag for self-healed key contact", "uid", existing.UID, "error", etagErr)
		}

		lastMod := existing.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
		result := &membershipservice.CreateKeyContactResult{
			KeyContact:   keyContactToResponse(existing),
			LastModified: &lastMod,
		}
		if etagVal != "" {
			result.Etag = &etagVal
		}
		return result, nil
	}

	input := model.KeyContactInput{
		Email:          &p.Email,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		Title:          derefStr(p.Title),
		MembershipUID:  p.MembershipUID,
		ProjectUID:     p.ProjectUID,
		AccountSFID:    p.B2bOrgUID, // B2bOrgUID in payload maps to AccountSFID in model
		Role:           &p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
	}

	kc, err := s.keyContactWriter.CreateKeyContact(ctx, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	s.publishKeyContactEvents(ctx, kc, indexerConstants.ActionCreated, constants.FGASyncUpdateAccessSubject, true)

	etagVal, etagErr := etag.LFXEtag(kc)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for key contact", "uid", kc.UID, "error", etagErr)
	}

	lastMod := kc.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	result := &membershipservice.CreateKeyContactResult{
		KeyContact:   keyContactToResponse(kc),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// UpdateKeyContact updates a key contact.
//
// ETag validation is performed here rather than forwarded to Salesforce because
// the SF sObject PATCH endpoint does not support the If-Match header (returns
// BAD_HEADER 400). We fetch the current record, validate the caller's If-Match
// against our computed ETag, then pass If-Unmodified-Since (SF LastModifiedDate)
// to the writer for SF-side concurrency protection.
func (s *membershipServicesrvc) UpdateKeyContact(ctx context.Context, p *membershipservice.UpdateKeyContactPayload) (*membershipservice.UpdateKeyContactResult, error) {
	// Fetch current record for ETag validation and capacity checking.
	current, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Validate If-Match against the current cached ETag before touching SF.
	// Salesforce PATCH rejects If-Match (BAD_HEADER), so ETag validation is done
	// here; the caller's If-Match never reaches the infrastructure layer.
	// Seed first/last name from the current record so ResolveOrCreateContact has
	// them available if a new Contact must be created for the incoming email address.
	// The update payload has no name fields — callers use a separate PATCH to the
	// contact-service for name-only changes.
	input := model.KeyContactInput{
		Role:           p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
		Title:          derefStr(p.Title),
		Email:          p.Email,
		AccountSFID:    current.B2BOrgUID, // needed for Contact resolution if email changes
		FirstName:      current.FirstName,
		LastName:       current.LastName,
	}

	if p.IfMatch != nil && *p.IfMatch != "" {
		currentETag, err := etag.LFXEtag(current)
		if err != nil {
			return nil, wrapError(ctx, pkgerrors.NewUnexpected("failed to compute etag", err))
		}
		if currentETag != *p.IfMatch {
			return nil, wrapError(ctx, pkgerrors.NewPreconditionFailed(
				fmt.Sprintf("key contact %s has been modified since last read (stale If-Match)", p.UID)))
		}
		// Translate to If-Unmodified-Since (SF LastModifiedDate) for the writer.
		input.IfUnmodifiedSince = current.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	}

	// Normalize and validate update (email normalization, capacity check on role change).
	if err := s.normalizeAndValidateUpdate(ctx, current, p); err != nil {
		return nil, wrapError(ctx, err)
	}

	kc, err := s.keyContactWriter.UpdateKeyContact(ctx, p.UID, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Skip publish on no-op updates to avoid spurious indexer/FGA events.
	currentETag, _ := etag.LFXEtag(current)
	etagVal, etagErr := etag.LFXEtag(kc)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for key contact", "uid", p.UID, "error", etagErr)
	}
	if currentETag != etagVal {
		s.publishKeyContactEvents(ctx, kc, indexerConstants.ActionUpdated, constants.FGASyncUpdateAccessSubject, true)
	}

	lastMod := kc.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	result := &membershipservice.UpdateKeyContactResult{
		KeyContact:   keyContactToResponse(kc),
		LastModified: &lastMod,
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	return result, nil
}

// DeleteKeyContact deletes a key contact.
//
// The DeleteKeyContactPayload does not carry MembershipUID, but the
// KeyContactWriter.DeleteKeyContact method requires it for cache invalidation.
// We fetch the contact first to extract the MembershipUID, then call the writer.
// After deletion, we publish delete events for both the indexer and FGA cleanup.
func (s *membershipServicesrvc) DeleteKeyContact(ctx context.Context, p *membershipservice.DeleteKeyContactPayload) error {
	// Fetch the current contact to extract MembershipUID for cache invalidation.
	kc, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return wrapError(ctx, err)
	}

	// If-Match supplied: reject if the ETag no longer matches the stored record.
	if p.IfMatch != nil && *p.IfMatch != "" {
		currentETag, etagErr := etag.LFXEtag(kc)
		if etagErr != nil {
			return wrapError(ctx, pkgerrors.NewUnexpected("failed to compute etag", etagErr))
		}
		if currentETag != *p.IfMatch {
			return wrapError(ctx, pkgerrors.NewPreconditionFailed(
				fmt.Sprintf("key contact %s has been modified since last read (stale If-Match)", p.UID)))
		}
	}

	// Delete the contact.
	if err := s.keyContactWriter.DeleteKeyContact(ctx, p.UID, kc.MembershipUID); err != nil {
		return wrapError(ctx, err)
	}

	s.publishKeyContactEvents(ctx, kc, indexerConstants.ActionDeleted, constants.FGASyncDeleteAccessSubject, false)

	return nil
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

// projectMembershipToResponse converts a domain ProjectMembership to the
// generated response type.
func projectMembershipToResponse(m *model.ProjectMembership) *membershipservice.ProjectMembershipResponse {
	resp := &membershipservice.ProjectMembershipResponse{
		UID: &m.UID,
	}

	// UID is always set (line above). All other fields are optional and populated
	// only when non-zero using the omit-zero pattern.
	if m.TierUID != "" {
		resp.TierUID = &m.TierUID
	}
	if m.ProjectUID != "" {
		resp.ProjectUID = &m.ProjectUID
	}
	if m.ProjectSlug != "" {
		resp.ProjectSlug = &m.ProjectSlug
	}
	if m.B2BOrgUID != "" {
		resp.B2bOrgUID = &m.B2BOrgUID
	}
	if m.Status != "" {
		resp.Status = &m.Status
	}
	if m.Year != "" {
		resp.Year = &m.Year
	}
	if m.Tier != "" {
		resp.Tier = &m.Tier
	}
	if m.AutoRenew {
		resp.AutoRenew = &m.AutoRenew
	}
	if m.RenewalType != "" {
		resp.RenewalType = &m.RenewalType
	}
	if m.Price != 0 {
		resp.Price = &m.Price
	}
	if m.AnnualFullPrice != 0 {
		resp.AnnualFullPrice = &m.AnnualFullPrice
	}
	if m.PaymentFrequency != "" {
		resp.PaymentFrequency = &m.PaymentFrequency
	}
	if m.PaymentTerms != "" {
		resp.PaymentTerms = &m.PaymentTerms
	}
	if m.AgreementDate != "" {
		resp.AgreementDate = &m.AgreementDate
	}
	if m.PurchaseDate != "" {
		resp.PurchaseDate = &m.PurchaseDate
	}
	if m.StartDate != "" {
		resp.StartDate = &m.StartDate
	}
	if m.EndDate != "" {
		resp.EndDate = &m.EndDate
	}
	if m.CompanyName != "" {
		resp.CompanyName = &m.CompanyName
	}
	if m.CompanyLogoURL != "" {
		resp.CompanyLogoURL = &m.CompanyLogoURL
	}
	if m.CompanyDomain != "" {
		resp.CompanyDomain = &m.CompanyDomain
	}
	if m.TierName != "" {
		resp.TierName = &m.TierName
	}
	if m.TierFamily != "" {
		resp.TierFamily = &m.TierFamily
	}
	if m.TierProductType != "" {
		resp.TierProductType = &m.TierProductType
	}

	createdAt := m.CreatedAt.UTC().Format(time.RFC3339)
	resp.CreatedAt = &createdAt
	updatedAt := m.UpdatedAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = &updatedAt

	return resp
}

// keyContactToResponse converts a domain KeyContact to the generated response type.
// Uses the omit-zero pattern: only non-zero fields are included.
func keyContactToResponse(kc *model.KeyContact) *membershipservice.ProjectKeyContactResponse {
	resp := &membershipservice.ProjectKeyContactResponse{
		UID: &kc.UID,
	}

	// All other fields are optional and populated only when non-zero.
	if kc.MembershipUID != "" {
		resp.MembershipUID = &kc.MembershipUID
	}
	if kc.TierUID != "" {
		resp.TierUID = &kc.TierUID
	}
	if kc.ProjectUID != "" {
		resp.ProjectUID = &kc.ProjectUID
	}
	if kc.B2BOrgUID != "" {
		resp.B2bOrgUID = &kc.B2BOrgUID
	}
	if kc.Role != "" {
		resp.Role = &kc.Role
	}
	if kc.Status != "" {
		resp.Status = &kc.Status
	}
	if kc.BoardMember {
		resp.BoardMember = &kc.BoardMember
	}
	if kc.PrimaryContact {
		resp.PrimaryContact = &kc.PrimaryContact
	}
	if kc.FirstName != "" {
		resp.FirstName = &kc.FirstName
	}
	if kc.LastName != "" {
		resp.LastName = &kc.LastName
	}
	if kc.Title != "" {
		resp.Title = &kc.Title
	}
	if kc.Email != "" {
		resp.Email = &kc.Email
	}
	if kc.CompanyName != "" {
		resp.CompanyName = &kc.CompanyName
	}
	if kc.CompanyLogoURL != "" {
		resp.CompanyLogoURL = &kc.CompanyLogoURL
	}
	if kc.CompanyDomain != "" {
		resp.CompanyDomain = &kc.CompanyDomain
	}

	createdAt := kc.CreatedAt.UTC().Format(time.RFC3339)
	resp.CreatedAt = &createdAt
	updatedAt := kc.UpdatedAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = &updatedAt

	return resp
}

// derefStr is a helper that dereferences a *string pointer, returning the empty
// string if the pointer is nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
	projectMembershipReader port.ProjectMembershipReader,
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
		projectMembershipReader:  projectMembershipReader,
		memberPublisher:          memberPublisher,
		globalOrgAdminTeamUID:    globalOrgAdminTeamUID,
	}
}
