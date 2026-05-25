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

	"github.com/google/uuid"
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
	storage                 port.MemberReader
	auth                    domain.Authenticator
	b2bOrgReader            port.B2BOrgReader
	projectMembershipReader port.ProjectMembershipReader
	b2bOrgSettingsReader    port.B2BOrgSettingsReader
	b2bOrgWriter            usecaseSvc.B2BOrgWriter
	keyContactWriter        usecaseSvc.KeyContactWriter
	orgSettingsWriter       usecaseSvc.OrgSettingsWriter
	backfillRunner          *usecaseSvc.Runner
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
	org, err := s.b2bOrgWriter.Create(ctx, p.Sfid)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

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
// ETag validation and no-op detection are handled by the orchestrator. When
// If-Match is absent the update is unconditional; when present and stale, 412
// is returned. A no-op (no payload changes) returns the current record as-is.
func (s *membershipServicesrvc) UpdateB2bOrg(ctx context.Context, p *membershipservice.UpdateB2bOrgPayload) (*membershipservice.UpdateB2bOrgResult, error) {
	input := payloadToB2BOrgInput(p)
	ifMatch := ""
	if p.IfMatch != nil {
		ifMatch = *p.IfMatch
	}
	org, err := s.b2bOrgWriter.Update(ctx, p.UID, input, ifMatch)
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

	// 404 (not 403) to avoid leaking existence of contacts in other memberships.
	if kc.MembershipUID != p.MembershipUID {
		return nil, wrapError(ctx, pkgerrors.NewNotFound(
			fmt.Sprintf("key contact %s not found in membership %s", p.UID, p.MembershipUID)))
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
	in := usecaseSvc.KeyContactCreateInput{
		MembershipUID:  p.MembershipUID,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		Email:          p.Email,
		Title:          p.Title,
		Role:           p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
	}
	kc, err := s.keyContactWriter.Create(ctx, in)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

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
// Cross-membership 404 check is performed here before delegating to the
// orchestrator — avoids leaking record existence across membership boundaries.
func (s *membershipServicesrvc) UpdateKeyContact(ctx context.Context, p *membershipservice.UpdateKeyContactPayload) (*membershipservice.UpdateKeyContactResult, error) {
	// 404 (not 403) to avoid leaking existence of contacts in other memberships.
	current, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	if current.MembershipUID != p.MembershipUID {
		return nil, wrapError(ctx, pkgerrors.NewNotFound(
			fmt.Sprintf("key contact %s not found in membership %s", p.UID, p.MembershipUID)))
	}

	in := usecaseSvc.KeyContactUpdateInput{
		MembershipUID:  p.MembershipUID,
		UID:            p.UID,
		Email:          p.Email,
		Title:          p.Title,
		Role:           p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
		IfMatch:        derefStr(p.IfMatch),
	}
	kc, err := s.keyContactWriter.Update(ctx, in)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	etagVal, etagErr := etag.LFXEtag(kc)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for key contact", "uid", p.UID, "error", etagErr)
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
// Cross-membership 404 check is performed here before delegating to the
// orchestrator — avoids leaking record existence across membership boundaries.
func (s *membershipServicesrvc) DeleteKeyContact(ctx context.Context, p *membershipservice.DeleteKeyContactPayload) error {
	kc, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return wrapError(ctx, err)
	}
	// 404 (not 403) to avoid leaking existence of contacts in other memberships.
	if kc.MembershipUID != p.MembershipUID {
		return wrapError(ctx, pkgerrors.NewNotFound(
			fmt.Sprintf("key contact %s not found in membership %s", p.UID, p.MembershipUID)))
	}

	in := usecaseSvc.KeyContactDeleteInput{
		MembershipUID: p.MembershipUID,
		UID:           p.UID,
		IfMatch:       derefStr(p.IfMatch),
	}
	if err := s.keyContactWriter.Delete(ctx, in); err != nil {
		return wrapError(ctx, err)
	}
	return nil
}

// ── Admin ─────────────────────────────────────────────────────────────────────

// AdminReindex validates the request, spawns an async backfill goroutine, and
// returns 202 Accepted with a run_id for log correlation.
func (s *membershipServicesrvc) AdminReindex(ctx context.Context, p *membershipservice.AdminReindexPayload) (*membershipservice.AdminReindexResult, error) {
	req, err := usecaseSvc.ValidateAndBuildRequest(p)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	runID := uuid.New().String()
	req.RunID = runID

	slog.InfoContext(ctx, "admin reindex accepted — search logs for run_id to track progress",
		"run_id", runID,
		"mode", string(usecaseSvc.ClassifyMode(req)),
		"dry_run", req.DryRun)

	if s.backfillRunner == nil {
		slog.WarnContext(ctx, "backfill runner not initialised — reindex skipped", "run_id", runID)
		return &membershipservice.AdminReindexResult{RunID: runID}, nil
	}
	// Fire-and-forget: the backfill runs independently of the HTTP request lifetime.
	// context.WithoutCancel prevents HTTP cancellation from killing a running page, but
	// the goroutine is not registered on the server's shutdown WaitGroup — a SIGTERM
	// during a large reindex will interrupt the run mid-flight (partial index, no error
	// logged). Accepted trade-off: /admin/reindex is a manual recovery tool and the
	// backfill can be re-triggered; graceful-shutdown integration is tracked as a
	// follow-up.
	go s.backfillRunner.Run(context.WithoutCancel(ctx), req)

	return &membershipservice.AdminReindexResult{RunID: runID}, nil
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
func keyContactToResponse(kc *model.KeyContact) *membershipservice.ProjectKeyContactResponse {
	resp := &membershipservice.ProjectKeyContactResponse{
		UID: &kc.UID,
	}

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

// derefStr dereferences a *string, returning "" when nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// payloadToB2BOrgInput maps an UpdateB2bOrgPayload to a model.B2BOrgInput.
func payloadToB2BOrgInput(p *membershipservice.UpdateB2bOrgPayload) model.B2BOrgInput {
	input := model.B2BOrgInput{}
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

// ── Org settings handlers ─────────────────────────────────────────────────────

// GetB2bOrgSettings returns the current access-control settings for a b2b_org.
// When no settings record exists yet it returns empty arrays — not a 404.
func (s *membershipServicesrvc) GetB2bOrgSettings(ctx context.Context, p *membershipservice.GetB2bOrgSettingsPayload) (*membershipservice.GetB2bOrgSettingsResult, error) {
	settings, _, err := s.b2bOrgSettingsReader.GetSettings(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	result := &membershipservice.GetB2bOrgSettingsResult{
		Settings: orgSettingsToResponse(settings),
	}
	etagVal, etagErr := etag.LFXEtag(settings)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org settings", "uid", p.UID, "error", etagErr)
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	if settings != nil {
		lastMod := settings.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
		result.LastModified = &lastMod
	}
	return result, nil
}

// UpdateB2bOrgSettings fully replaces the writers and/or auditors for a b2b_org.
// Nil writers/auditors = leave existing unchanged; explicit empty slice = clear.
func (s *membershipServicesrvc) UpdateB2bOrgSettings(ctx context.Context, p *membershipservice.UpdateB2bOrgSettingsPayload) (*membershipservice.UpdateB2bOrgSettingsResult, error) {
	now := time.Now().UTC()
	in := usecaseSvc.B2BOrgSettingsUpdate{
		OrgUID:  p.UID,
		IfMatch: derefStr(p.IfMatch),
	}
	if p.Writers != nil {
		in.Writers = orgUsersFromPayload(p.Writers, now)
	}
	if p.Auditors != nil {
		in.Auditors = orgUsersFromPayload(p.Auditors, now)
	}

	updated, err := s.orgSettingsWriter.Update(ctx, in)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	result := &membershipservice.UpdateB2bOrgSettingsResult{
		Settings: orgSettingsToResponse(updated),
	}
	etagVal, etagErr := etag.LFXEtag(updated)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for b2b org settings", "uid", p.UID, "error", etagErr)
	}
	if etagVal != "" {
		result.Etag = &etagVal
	}
	lastMod := updated.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
	result.LastModified = &lastMod
	return result, nil
}

// orgSettingsToResponse maps domain OrgSettings to the generated response type.
// A nil settings pointer is treated as empty (no settings stored yet).
func orgSettingsToResponse(s *model.B2BOrgSettings) *membershipservice.B2bOrgSettingsResponse {
	resp := &membershipservice.B2bOrgSettingsResponse{
		Writers:  []*membershipservice.OrgUser{},
		Auditors: []*membershipservice.OrgUser{},
	}
	if s == nil {
		return resp
	}
	for _, u := range s.Writers {
		resp.Writers = append(resp.Writers, orgUserToResponse(u))
	}
	for _, u := range s.Auditors {
		resp.Auditors = append(resp.Auditors, orgUserToResponse(u))
	}
	createdAt := s.CreatedAt.UTC().Format(time.RFC3339)
	resp.CreatedAt = &createdAt
	updatedAt := s.UpdatedAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = &updatedAt
	return resp
}

// orgUserToResponse maps a domain B2BOrgUser to the generated API type.
func orgUserToResponse(u model.B2BOrgUser) *membershipservice.OrgUser {
	out := &membershipservice.OrgUser{
		Email:     u.Email,
		InvitedAs: u.InvitedAs,
	}
	if u.Avatar != "" {
		out.Avatar = &u.Avatar
	}
	if u.Name != "" {
		out.Name = &u.Name
	}
	if u.Username != "" {
		out.Username = &u.Username
	}
	status := string(u.EffectiveStatus())
	out.InviteStatus = &status
	return out
}

// orgUsersFromPayload maps the API payload slice to domain B2BOrgUser slice, deriving
// InviteStatus: accepted when Username is set, pending otherwise.
func orgUsersFromPayload(users []*membershipservice.OrgUser, now time.Time) []model.B2BOrgUser {
	out := make([]model.B2BOrgUser, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		du := model.B2BOrgUser{
			Email:        u.Email,
			InvitedAs:    u.InvitedAs,
			InviteStatus: model.InviteStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if u.Avatar != nil {
			du.Avatar = *u.Avatar
		}
		if u.Name != nil {
			du.Name = *u.Name
		}
		if u.Username != nil && *u.Username != "" {
			du.Username = *u.Username
			du.InviteStatus = model.InviteStatusAccepted
		}
		out = append(out, du)
	}
	return out
}

// ── Constructor ───────────────────────────────────────────────────────────────

// NewMembershipService returns the membership-service implementation with
// injected dependencies.
func NewMembershipService(
	auth domain.Authenticator,
	storage port.MemberReader,
	b2bOrgReader port.B2BOrgReader,
	projectMshipR port.ProjectMembershipReader,
	b2bOrgSettingsReader port.B2BOrgSettingsReader,
	b2bOrgWriter usecaseSvc.B2BOrgWriter,
	keyContactWriter usecaseSvc.KeyContactWriter,
	orgSettingsWriter usecaseSvc.OrgSettingsWriter,
	backfillRunner *usecaseSvc.Runner,
) membershipservice.Service {
	return &membershipServicesrvc{
		storage:                 storage,
		auth:                    auth,
		b2bOrgReader:            b2bOrgReader,
		projectMembershipReader: projectMshipR,
		b2bOrgSettingsReader:    b2bOrgSettingsReader,
		b2bOrgWriter:            b2bOrgWriter,
		keyContactWriter:        keyContactWriter,
		orgSettingsWriter:       orgSettingsWriter,
		backfillRunner:          backfillRunner,
	}
}
