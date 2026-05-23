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
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/google/uuid"
	fgaConstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
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
	userReader               port.UserReader
	orgSettingsStorage       port.OrgSettingsStorage
	globalOrgAdminTeamUID    string
	backfillRunner           *BackfillRunner
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

	s.publishB2BOrgEvents(ctx, nil, org, indexerConstants.ActionCreated)

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

	// Always fetch current to compute reparenting diff and populate If-Unmodified-Since.
	current, err := s.b2bOrgReader.GetB2BOrg(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// SF PATCH rejects If-Match (returns BAD_HEADER), so ETag validation is done
	// here; we translate to If-Unmodified-Since for SF-side concurrency protection.
	if p.IfMatch != nil && *p.IfMatch != "" {
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
		etagVal, etagErr := etag.LFXEtag(current)
		if etagErr != nil {
			slog.WarnContext(ctx, "failed to compute etag for b2b org", "uid", p.UID, "error", etagErr)
		}
		lastMod := current.UpdatedAt.UTC().Format(constants.HTTPDateFormat)
		result := &membershipservice.UpdateB2bOrgResult{
			B2bOrg:       b2bOrgToResponse(current),
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

	s.publishB2BOrgEvents(ctx, current, org, indexerConstants.ActionUpdated)

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

func (s *membershipServicesrvc) publishKeyContactIndexer(ctx context.Context, kc *model.KeyContact, action indexerConstants.MessageAction) {
	publishKeyContactIndexer(ctx, s.memberPublisher, kc, action)
}

// publishProjectMembershipFGA emits a project_membership update_access FGA message
// so that parent b2b_org and project reference tuples exist before any key_contact
// member_put cascades through them. Errors are logged and swallowed — idempotent
// via fga-sync diff; repair via /admin/reindex if needed.
func (s *membershipServicesrvc) publishProjectMembershipFGA(ctx context.Context, pm *model.ProjectMembership) {
	msg := buildProjectMembershipFGAMessage(pm)
	if pubErr := s.memberPublisher.Access(ctx, constants.FGASyncUpdateAccessSubject, msg, false); pubErr != nil {
		slog.WarnContext(ctx, "project_membership FGA update_access publish failed",
			"membership_uid", pm.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// publishKeyContactFGAPut grants the key_contact relation on the parent
// project_membership to the given sub. No-op when sub is empty.
func (s *membershipServicesrvc) publishKeyContactFGAPut(ctx context.Context, membershipUID, sub string) {
	if sub == "" {
		return
	}
	msg := buildKeyContactFGAPutMessage(membershipUID, sub)
	if pubErr := s.memberPublisher.Access(ctx, fgaConstants.GenericMemberPutSubject, msg, false); pubErr != nil {
		slog.WarnContext(ctx, "key contact FGA put publish failed",
			"membership_uid", membershipUID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// publishKeyContactFGARemove revokes the key_contact relation on the parent
// project_membership from the given sub. Propagates errors (delete failures
// leave dangling permissions). No-op when sub is empty.
func (s *membershipServicesrvc) publishKeyContactFGARemove(ctx context.Context, membershipUID, sub string) error {
	if sub == "" {
		return nil
	}
	msg := buildKeyContactFGARemoveMessage(membershipUID, sub)
	return s.memberPublisher.Access(ctx, fgaConstants.GenericMemberRemoveSubject, msg, false)
}

// resolveSubForContact resolves the OIDC subject for the given email, using the
// persisted Username when already populated. Returns empty string on failure
// (fail-open: record creation/update proceeds without FGA grant).
func (s *membershipServicesrvc) resolveSubForContact(ctx context.Context, currentSub, email string) string {
	if currentSub != "" {
		return currentSub
	}
	if email == "" {
		return ""
	}
	sub, err := s.userReader.SubByEmail(ctx, email)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve user sub by email — FGA keycontact put will be skipped",
			"email", email,
			"error", err)
		return ""
	}
	return sub
}

// publishB2BOrgEvents fans out an indexer message, an FGA update-access message,
// and any b2b_org#parent + b2b_org#child reparenting messages for the given org.
// current is nil on create. Publish failures are swallowed and logged with
// publish_failed_for_backfill_repair=true — /admin/reindex recovers missed records.
func (s *membershipServicesrvc) publishB2BOrgEvents(ctx context.Context, current, org *model.B2BOrg, action indexerConstants.MessageAction) {
	publishB2BOrgIndexer(ctx, s.memberPublisher, org, action)

	orgAdminTeamUID := ""
	if action == indexerConstants.ActionCreated {
		orgAdminTeamUID = s.globalOrgAdminTeamUID
	}
	// writers, auditors, and membershipUIDs are fetched from the settings/index
	// store and passed when the settings PUT handler calls this path. For the
	// legacy create/update path they are nil, which leaves existing tuples for
	// those relations untouched (ExcludeRelations includes "membership").
	fgaMsg := buildB2BOrgFGAMessage(org, orgAdminTeamUID, nil, nil, nil)

	// Fetch child lists for affected parents so the builder can emit child-list
	// FGA tuples alongside the parent-ref message. Done synchronously before the
	// goroutine group since the slices are immutable inputs. Errors are logged and
	// result in nil slices — the builder skips child-list messages when nil.
	oldParentChildren, newParentChildren := s.fetchChildListsForReparent(ctx, current, org)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.memberPublisher.Access(gCtx, constants.FGASyncUpdateAccessSubject, fgaMsg, false)
	})

	// Emit b2b_org#parent and b2b_org#child FGA tuples when ParentUID changes.
	for _, reparentMsg := range buildB2BOrgReparentingMessages(current, org, oldParentChildren, newParentChildren) {
		msg := reparentMsg // capture loop var
		g.Go(func() error {
			return s.memberPublisher.Access(gCtx, constants.FGASyncUpdateAccessSubject, msg, false)
		})
	}

	if pubErr := g.Wait(); pubErr != nil {
		slog.WarnContext(ctx, "b2b org fga publish failed",
			"uid", org.UID,
			"error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// fetchChildListsForReparent computes the post-move child-UID slices for the
// old and new parent when a b2b_org's ParentUID changes. Returns (nil, nil)
// when parent is unchanged — the builder treats nil as "skip child-list message".
// Errors are logged and cause the affected slice to be nil (skip, don't corrupt).
func (s *membershipServicesrvc) fetchChildListsForReparent(ctx context.Context, current, org *model.B2BOrg) (oldChildren, newChildren []string) {
	oldParent := ""
	if current != nil {
		oldParent = current.ParentUID
	}
	newParent := org.ParentUID
	if oldParent == newParent {
		return nil, nil
	}

	g, gCtx := errgroup.WithContext(ctx)

	if oldParent != "" {
		g.Go(func() error {
			uids, err := s.b2bOrgReader.FetchChildUIDsByParentUID(gCtx, oldParent)
			if err != nil {
				slog.WarnContext(ctx, "failed to fetch children of old parent for FGA child-list update",
					"old_parent_uid", oldParent, "org_uid", org.UID, "error", err,
					"publish_failed_for_backfill_repair", true)
				return nil // swallow; nil slice = skip message
			}
			// Remove org from OldP's child list — it has moved away.
			for _, u := range uids {
				if u != org.UID {
					oldChildren = append(oldChildren, u)
				}
			}
			if oldChildren == nil {
				oldChildren = []string{} // non-nil empty = emit clear
			}
			return nil
		})
	}

	if newParent != "" {
		g.Go(func() error {
			uids, err := s.b2bOrgReader.FetchChildUIDsByParentUID(gCtx, newParent)
			if err != nil {
				slog.WarnContext(ctx, "failed to fetch children of new parent for FGA child-list update",
					"new_parent_uid", newParent, "org_uid", org.UID, "error", err,
					"publish_failed_for_backfill_repair", true)
				return nil // swallow; nil slice = skip message
			}
			// Ensure org is in NewP's list regardless of whether SF has processed the move.
			newChildren = uids
			if !slices.Contains(newChildren, org.UID) {
				newChildren = append(newChildren, org.UID)
			}
			return nil
		})
	}

	_ = g.Wait()
	return oldChildren, newChildren
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
	// Normalize and validate: lowercase email, trim names, check capacity, detect self-heal.
	// Self-heal lookup is scoped to p.MembershipUID so a match on membership A never
	// short-circuits a create on membership B (cross-membership leak prevention).
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

	// Derive b2b_org_uid and project_uid from the membership — callers no longer supply them.
	pm, _, err := s.projectMembershipReader.AssembleProjectMembership(ctx, p.MembershipUID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	input := model.KeyContactInput{
		Email:          &p.Email,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		Title:          derefStr(p.Title),
		MembershipUID:  p.MembershipUID,
		ProjectUID:     pm.ProjectUID,
		AccountSFID:    pm.B2BOrgUID,
		Role:           &p.Role,
		Status:         p.Status,
		BoardMember:    p.BoardMember,
		PrimaryContact: p.PrimaryContact,
	}

	kc, err := s.keyContactWriter.CreateKeyContact(ctx, input)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	// Emit project_membership update_access so the parent tuple exists before the
	// key_contact member_put. fga-sync diffs, so this is idempotent on re-create.
	s.publishProjectMembershipFGA(ctx, pm)

	// Resolve the OIDC sub and publish the keycontact FGA put. Indexer is always published.
	// Username is not persisted to Salesforce or NATS cache — it is always re-resolved
	// from the auth-service on subsequent reads (delete re-resolves via email by design).
	sub := s.resolveSubForContact(ctx, "", kc.Email)
	kc.Username = sub
	s.publishKeyContactIndexer(ctx, kc, indexerConstants.ActionCreated)
	s.publishKeyContactFGAPut(ctx, kc.MembershipUID, sub)

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

	// 404 (not 403) to avoid leaking existence of contacts in other memberships.
	if current.MembershipUID != p.MembershipUID {
		return nil, wrapError(ctx, pkgerrors.NewNotFound(
			fmt.Sprintf("key contact %s not found in membership %s", p.UID, p.MembershipUID)))
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
		MembershipUID:  p.MembershipUID, // required so the writer invalidates the key-contacts cache
	}

	currentETag, err := etag.LFXEtag(current)
	if err != nil {
		return nil, wrapError(ctx, pkgerrors.NewUnexpected("failed to compute etag", err))
	}

	if p.IfMatch != nil && *p.IfMatch != "" {
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
	etagVal, etagErr := etag.LFXEtag(kc)
	if etagErr != nil {
		slog.WarnContext(ctx, "failed to compute etag for key contact", "uid", p.UID, "error", etagErr)
	}
	if currentETag != etagVal {
		// Resolve Username before indexing so the indexed document carries the sub.
		emailChanging := p.Email != nil && !strings.EqualFold(*p.Email, current.Email)
		if emailChanging {
			// Paired FGA publish: put new sub first (no-access window avoided), then remove old.
			// Old sub: use persisted Username; fall back to resolving current.Email (may be empty if
			// the contact never had an Authelia account, in which case remove is skipped).
			newSub := s.resolveSubForContact(ctx, "", kc.Email)
			kc.Username = newSub
			s.publishKeyContactFGAPut(ctx, kc.MembershipUID, newSub)
			oldSub := s.resolveSubForContact(ctx, current.Username, current.Email)
			if oldSub != newSub {
				if pubErr := s.publishKeyContactFGARemove(ctx, kc.MembershipUID, oldSub); pubErr != nil {
					// Log at error severity (dangling permission), but do not propagate — the
					// Salesforce update already succeeded and returning an error here would
					// mislead callers into retrying a completed operation.
					slog.ErrorContext(ctx, "key contact FGA remove failed on email change — dangling permission",
						"uid", p.UID,
						"error", pubErr)
				}
			}
		} else {
			// Role/status/other-only update: re-resolve sub in case it was empty on create.
			sub := s.resolveSubForContact(ctx, current.Username, kc.Email)
			kc.Username = sub
			s.publishKeyContactFGAPut(ctx, kc.MembershipUID, sub)
		}
		s.publishKeyContactIndexer(ctx, kc, indexerConstants.ActionUpdated)
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
// We fetch the contact first to validate membership alignment and extract the
// persisted Username for the FGA remove. After deletion, the indexer record is
// removed and the FGA key_contact relation is revoked (error propagated — a
// delete without FGA cleanup leaves dangling permissions).
func (s *membershipServicesrvc) DeleteKeyContact(ctx context.Context, p *membershipservice.DeleteKeyContactPayload) error {
	// Fetch the current contact to validate membership alignment and get MembershipUID/Username.
	kc, err := s.storage.GetKeyContact(ctx, p.UID)
	if err != nil {
		return wrapError(ctx, err)
	}

	// 404 (not 403) to avoid leaking existence of contacts in other memberships.
	if kc.MembershipUID != p.MembershipUID {
		return wrapError(ctx, pkgerrors.NewNotFound(
			fmt.Sprintf("key contact %s not found in membership %s", p.UID, p.MembershipUID)))
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

	// Indexer delete: swallow errors (reindexable).
	s.publishKeyContactIndexer(ctx, kc, indexerConstants.ActionDeleted)

	// FGA remove: propagate errors — dangling permissions are not auto-repairable.
	sub := s.resolveSubForContact(ctx, kc.Username, kc.Email)
	if pubErr := s.publishKeyContactFGARemove(ctx, kc.MembershipUID, sub); pubErr != nil {
		slog.ErrorContext(ctx, "key contact FGA remove failed on delete — dangling permission",
			"uid", p.UID,
			"error", pubErr)
		return wrapError(ctx, pkgerrors.NewUnexpected("failed to revoke FGA access for deleted key contact", pubErr))
	}

	return nil
}

// ── Admin ─────────────────────────────────────────────────────────────────────

// AdminReindex validates the request, spawns an async backfill goroutine, and
// returns 202 Accepted with a run_id for log correlation.
func (s *membershipServicesrvc) AdminReindex(ctx context.Context, p *membershipservice.AdminReindexPayload) (*membershipservice.AdminReindexResult, error) {
	req, err := validateAndBuildBackfillRequest(p)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	runID := uuid.New().String()
	req.RunID = runID

	slog.InfoContext(ctx, "admin reindex accepted",
		"run_id", runID,
		"mode", string(classifyMode(req)),
		"dry_run", req.DryRun,
		"correlate via run_id in service logs", runID)

	if s.backfillRunner == nil {
		slog.WarnContext(ctx, "backfill runner not initialised — reindex skipped", "run_id", runID)
		return &membershipservice.AdminReindexResult{RunID: runID}, nil
	}
	go s.backfillRunner.Run(context.WithoutCancel(ctx), req)

	return &membershipservice.AdminReindexResult{RunID: runID}, nil
}

// validateAndBuildBackfillRequest validates the payload and returns a BackfillRequest.
func validateAndBuildBackfillRequest(p *membershipservice.AdminReindexPayload) (BackfillRequest, error) {
	validTypes := map[string]bool{
		entityTypeB2BOrg:            true,
		entityTypeProjectMembership: true,
		entityTypeKeyContact:        true,
	}

	// Validate types
	for _, t := range p.Types {
		if t == "membership_tier" {
			return BackfillRequest{}, pkgerrors.NewValidation(
				"membership_tier is not currently supported; remove it from types or omit types to reindex all supported types")
		}
		if !validTypes[t] {
			return BackfillRequest{}, pkgerrors.NewValidation(
				fmt.Sprintf("unknown type %q; supported types: b2b_org, project_membership, key_contact", t))
		}
	}

	// Mutual exclusivity: items vs types/since
	if len(p.Items) > 0 && (len(p.Types) > 0 || p.Since != nil) {
		return BackfillRequest{}, pkgerrors.NewValidation("items mode is mutually exclusive with types and since")
	}

	// Validate items
	for _, item := range p.Items {
		if item.Type == "membership_tier" {
			return BackfillRequest{}, pkgerrors.NewValidation(
				"membership_tier is not currently supported in items mode")
		}
		if !validTypes[item.Type] {
			return BackfillRequest{}, pkgerrors.NewValidation(
				fmt.Sprintf("unknown item type %q; supported types: b2b_org, project_membership, key_contact", item.Type))
		}
		if _, uuidErr := uuid.Parse(item.UID); uuidErr != nil {
			return BackfillRequest{}, pkgerrors.NewValidation(
				fmt.Sprintf("invalid UUID %q for item type %q", item.UID, item.Type))
		}
	}

	// Validate and normalise since
	var since *time.Time
	if p.Since != nil {
		t, parseErr := time.Parse(time.RFC3339, *p.Since)
		if parseErr != nil {
			return BackfillRequest{}, pkgerrors.NewValidation(
				fmt.Sprintf("since must be a valid RFC 3339 timestamp with an explicit zone offset (e.g. 2026-05-20T00:00:00Z): %v", parseErr))
		}
		// Reject naive timestamps (no zone) — time.Parse(RFC3339) already requires a zone,
		// but guard against edge cases by checking the zero offset is explicit.
		utc := t.UTC()
		since = &utc
	}

	// Convert items
	items := make([]ReindexItem, len(p.Items))
	for i, item := range p.Items {
		items[i] = ReindexItem{Type: item.Type, UID: item.UID}
	}

	return BackfillRequest{
		Types:  p.Types,
		Since:  since,
		Items:  items,
		DryRun: p.DryRun,
	}, nil
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

// ── Org settings handlers ─────────────────────────────────────────────────────

// GetB2bOrgSettings returns the current access-control settings for a b2b_org.
// When no settings record exists yet it returns empty arrays — not a 404.
func (s *membershipServicesrvc) GetB2bOrgSettings(ctx context.Context, p *membershipservice.GetB2bOrgSettingsPayload) (*membershipservice.GetB2bOrgSettingsResult, error) {
	settings, _, err := s.orgSettingsStorage.GetOrgSettings(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}
	return &membershipservice.GetB2bOrgSettingsResult{
		Settings: orgSettingsToResponse(settings),
	}, nil
}

// UpdateB2bOrgSettings fully replaces the writers and/or auditors for a b2b_org.
// Nil writers/auditors = leave existing unchanged; explicit empty slice = clear.
func (s *membershipServicesrvc) UpdateB2bOrgSettings(ctx context.Context, p *membershipservice.UpdateB2bOrgSettingsPayload) (*membershipservice.UpdateB2bOrgSettingsResult, error) {
	existing, revision, err := s.orgSettingsStorage.GetOrgSettings(ctx, p.UID)
	if err != nil {
		return nil, wrapError(ctx, err)
	}

	now := time.Now().UTC()
	updated := &model.OrgSettings{
		CreatedAt: now,
		UpdatedAt: now,
	}
	if existing != nil {
		updated.CreatedAt = existing.CreatedAt
		updated.Writers = existing.Writers
		updated.Auditors = existing.Auditors
	}

	if p.Writers != nil {
		updated.Writers = orgUsersFromPayload(p.Writers, now)
	}
	if p.Auditors != nil {
		updated.Auditors = orgUsersFromPayload(p.Auditors, now)
	}

	if err := s.orgSettingsStorage.PutOrgSettings(ctx, p.UID, updated, revision); err != nil {
		return nil, wrapError(ctx, err)
	}

	s.publishB2BOrgSettingsFGA(ctx, p.UID, updated)

	return &membershipservice.UpdateB2bOrgSettingsResult{
		Settings: orgSettingsToResponse(updated),
	}, nil
}

// publishB2BOrgSettingsFGA emits a b2b_org update_access FGA message with the
// active writers and auditors from settings. Errors are logged and swallowed —
// fga-sync can be repaired via /admin/reindex.
func (s *membershipServicesrvc) publishB2BOrgSettingsFGA(ctx context.Context, orgUID string, settings *model.OrgSettings) {
	org, err := s.b2bOrgReader.GetB2BOrg(ctx, orgUID)
	if err != nil {
		slog.WarnContext(ctx, "could not fetch org for settings FGA publish — skipping",
			"uid", orgUID, "error", err,
			"publish_failed_for_backfill_repair", true)
		return
	}
	fgaMsg := buildB2BOrgFGAMessage(
		org,
		"",
		settings.ActiveWriterUsernames(),
		settings.ActiveAuditorUsernames(),
		nil,
	)
	if pubErr := s.memberPublisher.Access(ctx, constants.FGASyncUpdateAccessSubject, fgaMsg, false); pubErr != nil {
		slog.WarnContext(ctx, "b2b org settings FGA publish failed",
			"uid", orgUID, "error", pubErr,
			"publish_failed_for_backfill_repair", true)
	}
}

// orgSettingsToResponse maps domain OrgSettings to the generated response type.
// A nil settings pointer is treated as empty (no settings stored yet).
func orgSettingsToResponse(s *model.OrgSettings) *membershipservice.B2bOrgSettingsResponse {
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

// orgUserToResponse maps a domain OrgUser to the generated API type.
func orgUserToResponse(u model.OrgUser) *membershipservice.OrgUser {
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
	status := string(u.InviteStatus)
	out.InviteStatus = &status
	return out
}

// orgUsersFromPayload maps the API payload slice to domain OrgUser slice, deriving
// InviteStatus: accepted when Username is set, pending otherwise.
func orgUsersFromPayload(users []*membershipservice.OrgUser, now time.Time) []model.OrgUser {
	out := make([]model.OrgUser, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		du := model.OrgUser{
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
	readMemberUseCase usecaseSvc.MemberReader,
	storage port.MemberReader,
	authenticator domain.Authenticator,
	keyContactWriter port.KeyContactWriter,
	b2bOrgReader port.B2BOrgReader,
	b2bOrgWriter port.B2BOrgWriter,
	projectMembershipReader port.ProjectMembershipReader,
	memberPublisher port.MemberPublisher,
	userReader port.UserReader,
	globalOrgAdminTeamUID string,
	backfillRunner *BackfillRunner,
	orgSettingsStorage port.OrgSettingsStorage,
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
		userReader:               userReader,
		orgSettingsStorage:       orgSettingsStorage,
		globalOrgAdminTeamUID:    globalOrgAdminTeamUID,
		backfillRunner:           backfillRunner,
	}
}
