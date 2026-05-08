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

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
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

// ── B2B Organizations (Stubs) ─────────────────────────────────────────────────

// GetB2bOrg retrieves a single B2B organization by UID.
func (s *membershipServicesrvc) GetB2bOrg(ctx context.Context, p *membershipservice.GetB2bOrgPayload) (*membershipservice.GetB2bOrgResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("get-b2b-org not implemented"))
}

// CreateB2bOrg creates a new B2B organization.
func (s *membershipServicesrvc) CreateB2bOrg(ctx context.Context, p *membershipservice.CreateB2bOrgPayload) (*membershipservice.CreateB2bOrgResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("create-b2b-org not implemented"))
}

// UpdateB2bOrg updates a B2B organization.
func (s *membershipServicesrvc) UpdateB2bOrg(ctx context.Context, p *membershipservice.UpdateB2bOrgPayload) (*membershipservice.UpdateB2bOrgResult, error) {
	return nil, wrapError(ctx, pkgerrors.NewNotImplemented("update-b2b-org not implemented"))
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

// ── Constructor ───────────────────────────────────────────────────────────────

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
