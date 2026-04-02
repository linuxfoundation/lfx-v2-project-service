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

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	sfsvc "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"

	"goa.design/goa/v3/security"
)

// membershipServicesrvc implements the generated membershipservice.Service interface.
type membershipServicesrvc struct {
	memberReaderOrchestrator usecaseSvc.MemberReader
	storage                  port.MemberReader
	auth                     domain.Authenticator
	keyContactWriter         port.KeyContactWriter
	b2bOrgReader             port.B2BOrgReader
}

// JWTAuth implements the authorization logic for service "membership-service".
func (s *membershipServicesrvc) JWTAuth(ctx context.Context, token string, _ *security.JWTScheme) (context.Context, error) {
	principal, err := s.auth.ParsePrincipal(ctx, token, slog.Default())
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, constants.PrincipalContextID, principal), nil
}

// ── Tiers ────────────────────────────────────────────────────────────────────

// ListProjectTiers lists all membership tiers for a given project SFID.
func (s *membershipServicesrvc) ListProjectTiers(ctx context.Context, p *membershipservice.ListProjectTiersPayload) (*membershipservice.ListProjectTiersResult, error) {
	slog.DebugContext(ctx, "membershipService.list-project-tiers", "project_uid", p.ProjectUID)

	tiers, err := s.memberReaderOrchestrator.ListTiersForProject(ctx, *p.ProjectUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	responses := make([]*membershipservice.MembershipTierResponse, 0, len(tiers))
	for _, t := range tiers {
		responses = append(responses, convertTierToResponse(t))
	}

	return &membershipservice.ListProjectTiersResult{Tiers: responses}, nil
}

// GetProjectTier retrieves a single membership tier by UID.
func (s *membershipServicesrvc) GetProjectTier(ctx context.Context, p *membershipservice.GetProjectTierPayload) (*membershipservice.GetProjectTierResult, error) {
	slog.DebugContext(ctx, "membershipService.get-project-tier",
		"project_uid", p.ProjectUID,
		"tier_uid", p.TierUID,
	)

	tier, err := s.memberReaderOrchestrator.GetTier(ctx, *p.TierUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// tier.ProjectUID and *p.ProjectUID are both v2 UUIDs; the comparison is safe
	// by construction after the ProjectResolver fixes populate ProjectUID from the
	// project-service slug_to_uid RPC rather than from the Salesforce SFID.
	if tier.ProjectUID != *p.ProjectUID {
		return nil, wrapError(ctx, errNotFound("tier not found for this project"))
	}

	return &membershipservice.GetProjectTierResult{Tier: convertTierToResponse(tier)}, nil
}

// ── Memberships ───────────────────────────────────────────────────────────────

// ListProjectMemberships lists a single page of memberships for a given project,
// with optional SOQL-pushable filters, sort order, and cursor-based pagination.
func (s *membershipServicesrvc) ListProjectMemberships(ctx context.Context, p *membershipservice.ListProjectMembershipsPayload) (*membershipservice.ListProjectMembershipsResult, error) {
	var encodedPageToken string
	if p.PageToken != nil {
		encodedPageToken = *p.PageToken
	}

	// Decode the opaque consumer-facing cursor token. An empty token means
	// "start from the first page"; a non-empty token must be a valid
	// base64url-encoded PageCursor JSON blob.
	cursor, err := sfsvc.DecodeCursor(encodedPageToken)
	if err != nil {
		return nil, wrapError(ctx, fmt.Errorf("invalid page_token: %w", err))
	}
	// Re-encode to the canonical raw token string that MembershipFilters.PageToken
	// expects — the SOQL layer decodes it again via DecodeCursor.
	rawPageToken := encodedPageToken
	_ = cursor // cursor validated; rawPageToken passed through as-is

	slog.DebugContext(ctx, "membershipService.list-project-memberships",
		"project_uid", p.ProjectUID,
		"page_size", p.PageSize,
		"sort", p.Sort,
		"page_token_set", rawPageToken != "",
		"filter", p.Filter,
		"search_name", p.SearchName,
	)

	// Parse SOQL-pushable filters. Status is not exposed — the base query is
	// hardcoded to active members only. Sort order, page token, and company
	// name search are threaded directly into MembershipFilters so they reach
	// the SOQL layer.
	soqlFilters := parseMembershipFilters(p.Filter)
	soqlFilters.SortOrder = parseSortOrder(p.Sort)
	soqlFilters.PageToken = rawPageToken
	if p.SearchName != nil && *p.SearchName != "" {
		// Normalise to lowercase: SOQL LIKE is case-insensitive, so "Google"
		// and "google" produce identical results. Lowercasing here ensures
		// both values map to the same NATS KV cache key.
		soqlFilters.CompanyNameSearch = strings.ToLower(*p.SearchName)
	}

	memberPage, err := s.memberReaderOrchestrator.ListMembershipsForProject(ctx, *p.ProjectUID, soqlFilters, p.PageSize)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Apply remaining in-process filters (relationship fields not pushable to
	// SOQL WHERE). Company name search is now handled by SOQL LIKE, so it is
	// not passed to filterMemberships. Other non-SOQL fields (tier_name,
	// project_slug) still apply in-process if present in the filter string.
	inProcessFilters := parseFilters(p.Filter)
	delete(inProcessFilters, "tier_uid")
	memberships := filterMemberships(memberPage.Memberships, inProcessFilters, "")

	responses := make([]*membershipservice.ProjectMembershipResponse, 0, len(memberships))
	for _, m := range memberships {
		responses = append(responses, convertProjectMembershipToResponse(m))
	}

	metadata := &membershipservice.ListMetadata{}
	if memberPage.TotalSize > 0 {
		total := memberPage.TotalSize
		metadata.TotalSize = &total
	}
	if memberPage.NextPageToken != "" {
		// NextPageToken from FetchMembershipPage is already an EncodeCursor
		// base64url blob — pass it through directly.
		tok := memberPage.NextPageToken
		metadata.NextPageToken = &tok
	}

	return &membershipservice.ListProjectMembershipsResult{
		Memberships: responses,
		Metadata:    metadata,
	}, nil
}

// GetProjectMembership retrieves a single membership by UID within a project.
func (s *membershipServicesrvc) GetProjectMembership(ctx context.Context, p *membershipservice.GetProjectMembershipPayload) (*membershipservice.GetProjectMembershipResult, error) {
	slog.DebugContext(ctx, "membershipService.get-project-membership",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
	)

	membership, err := s.memberReaderOrchestrator.GetMembership(ctx, *p.MembershipUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Verify the membership belongs to the requested project. membership.ProjectUID
	// and *p.ProjectUID are both v2 UUIDs; the comparison is safe by construction
	// after the ProjectResolver fixes populate ProjectUID from the project-service
	// slug_to_uid RPC rather than from the Salesforce SFID.
	if membership.ProjectUID != *p.ProjectUID {
		return nil, wrapError(ctx, fmt.Errorf("membership %s does not belong to project %s: %w",
			*p.MembershipUID, *p.ProjectUID, errNotFound("membership not found for this project")))
	}

	// Revision is not meaningful for SOQL-backed records; send a static sentinel.
	etag := "0"
	return &membershipservice.GetProjectMembershipResult{
		Membership: convertProjectMembershipToResponse(membership),
		Etag:       &etag,
	}, nil
}

// ── Key contacts ─────────────────────────────────────────────────────────────

// CreateMembershipKeyContact creates a new key contact for a membership.
func (s *membershipServicesrvc) CreateMembershipKeyContact(ctx context.Context, p *membershipservice.CreateMembershipKeyContactPayload) (*membershipservice.CreateMembershipKeyContactResult, error) {
	slog.DebugContext(ctx, "membershipService.create-membership-key-contact",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
	)

	// Validate required identity fields.
	if p.Email == "" || p.FirstName == "" || p.LastName == "" {
		return nil, wrapError(ctx, pkgerrors.NewValidation("email, first_name, and last_name are required", nil))
	}

	// Verify the membership belongs to the requested project.
	membership, err := s.memberReaderOrchestrator.GetMembership(ctx, *p.MembershipUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	if membership.ProjectUID != *p.ProjectUID {
		return nil, wrapError(ctx, errNotFound("membership not found for this project"))
	}

	input := model.KeyContactInput{
		Email:          p.Email,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		MembershipUID:  *p.MembershipUID,
		ProjectUID:     *p.ProjectUID,
		AccountSFID:    membership.AccountSFID,
		Role:           p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
	}
	if p.Title != nil {
		input.Title = *p.Title
	}

	contact, err := s.keyContactWriter.CreateKeyContact(ctx, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	return &membershipservice.CreateMembershipKeyContactResult{
		Contact: convertProjectKeyContactToResponse(contact),
	}, nil
}

// UpdateMembershipKeyContact updates the mutable fields of an existing key contact.
func (s *membershipServicesrvc) UpdateMembershipKeyContact(ctx context.Context, p *membershipservice.UpdateMembershipKeyContactPayload) (*membershipservice.UpdateMembershipKeyContactResult, error) {
	slog.DebugContext(ctx, "membershipService.update-membership-key-contact",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
		"contact_uid", p.ContactUID,
	)

	// Verify the contact belongs to the requested membership.
	existing, err := s.memberReaderOrchestrator.GetKeyContact(ctx, *p.ContactUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	if existing.MembershipUID != *p.MembershipUID {
		return nil, wrapError(ctx, errNotFound("key contact not found for this membership"))
	}

	input := model.KeyContactInput{
		MembershipUID:  *p.MembershipUID,
		ProjectUID:     *p.ProjectUID,
		Role:           p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
	}

	contact, err := s.keyContactWriter.UpdateKeyContact(ctx, *p.ContactUID, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	return &membershipservice.UpdateMembershipKeyContactResult{
		Contact: convertProjectKeyContactToResponse(contact),
	}, nil
}

// DeleteMembershipKeyContact soft-deletes a key contact from a membership.
func (s *membershipServicesrvc) DeleteMembershipKeyContact(ctx context.Context, p *membershipservice.DeleteMembershipKeyContactPayload) error {
	slog.DebugContext(ctx, "membershipService.delete-membership-key-contact",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
		"contact_uid", p.ContactUID,
	)

	// Fetch to verify ownership and obtain the MembershipUID for cache invalidation.
	existing, err := s.memberReaderOrchestrator.GetKeyContact(ctx, *p.ContactUID)
	if err != nil {
		return wrapError(ctx, err)
	}
	if existing.MembershipUID != *p.MembershipUID {
		return wrapError(ctx, errNotFound("key contact not found for this membership"))
	}

	if err := s.keyContactWriter.DeleteKeyContact(ctx, *p.ContactUID, existing.MembershipUID); err != nil {
		return wrapError(ctx, err)
	}

	return nil
}

// ListMembershipKeyContacts lists all key contacts for a given membership UID.
func (s *membershipServicesrvc) ListMembershipKeyContacts(ctx context.Context, p *membershipservice.ListMembershipKeyContactsPayload) (*membershipservice.ListMembershipKeyContactsResult, error) {
	slog.DebugContext(ctx, "membershipService.list-membership-key-contacts",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
	)

	contacts, err := s.memberReaderOrchestrator.ListKeyContactsForMembership(ctx, *p.MembershipUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	responses := make([]*membershipservice.ProjectKeyContactResponse, 0, len(contacts))
	for _, c := range contacts {
		responses = append(responses, convertProjectKeyContactToResponse(c))
	}

	return &membershipservice.ListMembershipKeyContactsResult{Contacts: responses}, nil
}

// GetMembershipKeyContact retrieves a single key contact by UID within a
// membership.
func (s *membershipServicesrvc) GetMembershipKeyContact(ctx context.Context, p *membershipservice.GetMembershipKeyContactPayload) (*membershipservice.GetMembershipKeyContactResult, error) {
	slog.DebugContext(ctx, "membershipService.get-membership-key-contact",
		"project_uid", p.ProjectUID,
		"membership_uid", p.MembershipUID,
		"contact_uid", p.ContactUID,
	)

	contact, err := s.memberReaderOrchestrator.GetKeyContact(ctx, *p.ContactUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Verify the contact belongs to the requested membership.
	if contact.MembershipUID != *p.MembershipUID {
		return nil, wrapError(ctx, errNotFound("key contact not found for this membership"))
	}

	return &membershipservice.GetMembershipKeyContactResult{Contact: convertProjectKeyContactToResponse(contact)}, nil
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

// DebugVars returns the expvar debug variables as a JSON object. The output
// format is identical to the standard expvar HTTP handler (expanded with
// newlines between keys): each key is JSON-quoted, and each value is rendered
// using its String() method, which already returns valid JSON for all built-in
// expvar types (Int, Float, String, Map, Func). This avoids registering the
// default expvar handler on the default mux while still serving through the
// Goa-generated HTTP stack.
func (s *membershipServicesrvc) DebugVars(_ context.Context) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	first := true
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		// json.Marshal produces a properly escaped JSON string for the key.
		key, _ := json.Marshal(kv.Key)
		fmt.Fprintf(&buf, "%s: %s", key, kv.Value.String())
	})
	buf.WriteString("\n}\n")
	return buf.Bytes(), nil
}

// ── Constructor ───────────────────────────────────────────────────────────────

// ── B2B Organizations ─────────────────────────────────────────────────────────

// ListB2bOrgs searches and lists B2B organizations (Salesforce Accounts) by
// name with cursor-based pagination.
func (s *membershipServicesrvc) ListB2bOrgs(ctx context.Context, p *membershipservice.ListB2bOrgsPayload) (*membershipservice.ListB2bOrgsResult, error) {
	var encodedPageToken string
	if p.PageToken != nil {
		encodedPageToken = *p.PageToken
	}

	cursor, err := sfsvc.DecodeCursor(encodedPageToken)
	if err != nil {
		return nil, wrapError(ctx, fmt.Errorf("invalid page_token: %w", err))
	}
	_ = cursor // Cursor validated; token passed through as-is.

	slog.DebugContext(ctx, "membershipService.list-b2b-orgs",
		"page_size", p.PageSize,
		"sort", p.Sort,
		"page_token_set", encodedPageToken != "",
		"search_name", p.SearchName,
	)

	filters := model.B2BOrgFilters{
		SortOrder: parseSortOrder(p.Sort),
		PageToken: encodedPageToken,
	}
	if p.SearchName != nil && *p.SearchName != "" {
		filters.NameSearch = strings.ToLower(*p.SearchName)
	}

	orgPage, err := s.b2bOrgReader.SearchB2BOrgs(ctx, filters, p.PageSize)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	responses := make([]*membershipservice.B2bOrgResponse, 0, len(orgPage.Orgs))
	for _, org := range orgPage.Orgs {
		responses = append(responses, convertB2BOrgToResponse(org))
	}

	metadata := &membershipservice.ListMetadata{}
	if orgPage.TotalSize > 0 {
		total := orgPage.TotalSize
		metadata.TotalSize = &total
	}
	if orgPage.NextPageToken != "" {
		tok := orgPage.NextPageToken
		metadata.NextPageToken = &tok
	}

	return &membershipservice.ListB2bOrgsResult{
		Orgs:     responses,
		Metadata: metadata,
	}, nil
}

// ListB2bOrgMemberships lists all memberships (Assets) across all projects for
// a given B2B organization UID, with cursor-based pagination and filters.
func (s *membershipServicesrvc) ListB2bOrgMemberships(ctx context.Context, p *membershipservice.ListB2bOrgMembershipsPayload) (*membershipservice.ListB2bOrgMembershipsResult, error) {
	var encodedPageToken string
	if p.PageToken != nil {
		encodedPageToken = *p.PageToken
	}

	cursor, err := sfsvc.DecodeCursor(encodedPageToken)
	if err != nil {
		return nil, wrapError(ctx, fmt.Errorf("invalid page_token: %w", err))
	}
	_ = cursor // Cursor validated; token passed through as-is.

	slog.DebugContext(ctx, "membershipService.list-b2b-org-memberships",
		"b2b_org_uid", p.B2bOrgUID,
		"page_size", p.PageSize,
		"sort", p.Sort,
		"page_token_set", encodedPageToken != "",
		"filter", p.Filter,
		"search_name", p.SearchName,
	)

	soqlFilters := parseMembershipFilters(p.Filter)
	soqlFilters.SortOrder = parseSortOrder(p.Sort)
	soqlFilters.PageToken = encodedPageToken
	if p.SearchName != nil && *p.SearchName != "" {
		soqlFilters.CompanyNameSearch = strings.ToLower(*p.SearchName)
	}

	memberPage, err := s.memberReaderOrchestrator.ListMembershipsForB2BOrg(ctx, *p.B2bOrgUID, soqlFilters, p.PageSize)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Apply remaining in-process filters (relationship fields not pushable to
	// SOQL WHERE). Tier UID and company name search are handled by SOQL.
	inProcessFilters := parseFilters(p.Filter)
	delete(inProcessFilters, "tier_uid")
	memberships := filterMemberships(memberPage.Memberships, inProcessFilters, "")

	responses := make([]*membershipservice.ProjectMembershipResponse, 0, len(memberships))
	for _, m := range memberships {
		responses = append(responses, convertProjectMembershipToResponse(m))
	}

	metadata := &membershipservice.ListMetadata{}
	if memberPage.TotalSize > 0 {
		total := memberPage.TotalSize
		metadata.TotalSize = &total
	}
	if memberPage.NextPageToken != "" {
		tok := memberPage.NextPageToken
		metadata.NextPageToken = &tok
	}

	return &membershipservice.ListB2bOrgMembershipsResult{
		Memberships: responses,
		Metadata:    metadata,
	}, nil
}

// NewMembershipService returns the membership-service implementation with
// injected dependencies.
func NewMembershipService(
	readMemberUseCase usecaseSvc.MemberReader,
	storage port.MemberReader,
	authenticator domain.Authenticator,
	keyContactWriter port.KeyContactWriter,
	b2bOrgReader port.B2BOrgReader,
) membershipservice.Service {
	return &membershipServicesrvc{
		memberReaderOrchestrator: readMemberUseCase,
		storage:                  storage,
		auth:                     authenticator,
		keyContactWriter:         keyContactWriter,
		b2bOrgReader:             b2bOrgReader,
	}
}

// ── Filtering helpers ─────────────────────────────────────────────────────────

// filterMemberships applies in-process filter and search predicates to a slice
// of ProjectMembership domain records. Filtering mirrors the documented query
// param semantics: case-insensitive exact or substring match per field.
func filterMemberships(memberships []*model.ProjectMembership, filters map[string]string, search string) []*model.ProjectMembership {
	if len(filters) == 0 && search == "" {
		return memberships
	}

	result := make([]*model.ProjectMembership, 0, len(memberships))
	for _, m := range memberships {
		if matchesMembership(m, filters, search) {
			result = append(result, m)
		}
	}
	return result
}

// matchesMembership reports whether m satisfies all filter predicates and the
// free-text search term.
func matchesMembership(m *model.ProjectMembership, filters map[string]string, search string) bool {
	if search != "" {
		lower := strings.ToLower(search)
		if !strings.Contains(strings.ToLower(m.CompanyName), lower) &&
			!strings.Contains(strings.ToLower(m.ProjectSlug), lower) &&
			!strings.Contains(strings.ToLower(m.Tier), lower) &&
			!strings.Contains(strings.ToLower(m.TierName), lower) {
			return false
		}
	}

	for key, value := range filters {
		switch strings.ToLower(key) {
		case "status":
			if !strings.EqualFold(m.Status, value) {
				return false
			}
		case "tier":
			if !strings.EqualFold(m.Tier, value) {
				return false
			}
		case "year":
			if m.Year != value {
				return false
			}
		case "membership_type":
			if !strings.EqualFold(m.MembershipType, value) {
				return false
			}
		case "company_name":
			if !strings.Contains(strings.ToLower(m.CompanyName), strings.ToLower(value)) {
				return false
			}
		case "project_slug":
			if !strings.EqualFold(m.ProjectSlug, value) {
				return false
			}
		case "tier_name":
			if !strings.Contains(strings.ToLower(m.TierName), strings.ToLower(value)) {
				return false
			}
		}
	}

	return true
}

// parseMembershipFilters extracts the SOQL-pushable filter field (tier_uid)
// from the semicolon-separated filter string into a MembershipFilters struct.
// Status is not exposed — all membership queries are hardcoded to active members.
// SortOrder and PageToken are populated by the caller after this call returns.
// Unrecognised keys are ignored here; they are handled by the in-process
// filterMemberships call.
func parseMembershipFilters(filter *string) model.MembershipFilters {
	raw := parseFilters(filter)
	return model.MembershipFilters{
		TierUID: raw["tier_uid"],
	}
}

// parseSortOrder converts the raw sort query-parameter string to a model
// SortOrder. Unrecognised or empty values default to SortOrderDefault.
func parseSortOrder(raw string) model.SortOrder {
	switch model.SortOrder(raw) {
	case model.SortOrderName, model.SortOrderNewest, model.SortOrderLastModified:
		return model.SortOrder(raw)
	default:
		return model.SortOrderDefault
	}
}

// parseFilters parses a semicolon-separated "key=value;key=value" filter string
// into a map. Empty or nil input returns an empty map.
func parseFilters(filter *string) map[string]string {
	filters := make(map[string]string)
	if filter == nil || *filter == "" {
		return filters
	}

	for _, pair := range strings.Split(*filter, ";") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" && value != "" {
				filters[key] = value
			}
		}
	}

	return filters
}
