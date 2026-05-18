// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// normalizeAndValidateCreate lowercases/trims email+names, enforces required fields,
// checks per-role capacity, and detects idempotent-create duplicates. Returns an
// existing *model.KeyContact when a matching active (role+email) record is found —
// the caller must return it directly without calling the writer (self-heal).
func (s *membershipServicesrvc) normalizeAndValidateCreate(
	ctx context.Context,
	p *membershipservice.CreateKeyContactPayload,
) (*model.KeyContact, error) {
	p.Email = strings.ToLower(strings.TrimSpace(p.Email))
	p.FirstName = strings.TrimSpace(p.FirstName)
	p.LastName = strings.TrimSpace(p.LastName)

	if p.Email == "" {
		return nil, pkgerrors.NewValidation("email is required")
	}
	if p.FirstName == "" {
		return nil, pkgerrors.NewValidation("first_name is required")
	}
	if p.LastName == "" {
		return nil, pkgerrors.NewValidation("last_name is required")
	}

	existing, err := s.storage.ListKeyContactsForMembership(ctx, p.MembershipUID)
	if err != nil {
		return nil, err
	}

	activePerRole := make(map[string]int)
	for _, kc := range existing {
		if !strings.EqualFold(kc.Status, constants.RoleStatusInactive) {
			activePerRole[kc.Role]++
			// Self-heal: same role + same email already active → return as-is.
			if kc.Role == p.Role && strings.EqualFold(kc.Email, p.Email) {
				return kc, nil
			}
		}
	}

	if limit, ok := constants.KeyContactRoleLimits[p.Role]; ok && activePerRole[p.Role] >= limit {
		return nil, pkgerrors.NewConflict(
			fmt.Sprintf("%s is limited to %d per membership", p.Role, limit))
	}

	return nil, nil
}

// normalizeAndValidateUpdate lowercases email (if set), then enforces duplicate
// detection whenever email or role changes, and capacity only when role changes
// (legacy guard pattern: capacity re-applies only on a role change).
func (s *membershipServicesrvc) normalizeAndValidateUpdate(
	ctx context.Context,
	current *model.KeyContact,
	p *membershipservice.UpdateKeyContactPayload,
) error {
	// Mutates in-place (not replacing the pointer) so the caller's input.Email —
	// set from p.Email before this call — also sees the canonical value.
	if p.Email != nil {
		*p.Email = strings.ToLower(strings.TrimSpace(*p.Email))
	}

	newRole := current.Role
	if p.Role != nil {
		newRole = *p.Role
	}

	roleChanging := newRole != current.Role
	emailChanging := p.Email != nil && !strings.EqualFold(*p.Email, current.Email)
	// Re-activating an inactive contact can exceed capacity or create a (role,email) collision.
	statusActivating := p.Status != nil &&
		strings.EqualFold(*p.Status, constants.RoleStatusActive) &&
		!strings.EqualFold(current.Status, constants.RoleStatusActive)

	// Nothing that affects uniqueness or capacity is changing — skip sibling scan.
	if !roleChanging && !emailChanging && !statusActivating {
		return nil
	}

	existing, err := s.storage.ListKeyContactsForMembership(ctx, current.MembershipUID)
	if err != nil {
		return err
	}

	effectiveEmail := current.Email
	if p.Email != nil {
		effectiveEmail = *p.Email
	}

	// Single pass: detect (role, email) duplicates and, when role is changing,
	// count active siblings for capacity enforcement.
	// Inactive siblings and the current record are excluded from both checks.
	activeCount := 0
	for _, kc := range existing {
		if kc.UID == current.UID || strings.EqualFold(kc.Status, constants.RoleStatusInactive) {
			continue
		}
		if kc.Role == newRole {
			if strings.EqualFold(kc.Email, effectiveEmail) {
				return pkgerrors.NewConflict(
					fmt.Sprintf("key contact already exists for email %s with role %s",
						effectiveEmail, newRole))
			}
			activeCount++
		}
	}

	// Capacity is re-enforced when the role changes or an inactive contact is being re-activated.
	if roleChanging || statusActivating {
		if limit, ok := constants.KeyContactRoleLimits[newRole]; ok && activeCount >= limit {
			return pkgerrors.NewConflict(
				fmt.Sprintf("%s is limited to %d per membership", newRole, limit))
		}
	}

	return nil
}
