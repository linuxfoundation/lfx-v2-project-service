// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// normalizeAndValidateCreate lowercases/trims email+names, enforces required fields,
// checks per-role capacity, and detects idempotent-create duplicates. Returns an
// existing *model.KeyContact when a matching active (role+email) record is found —
// the caller must return it directly without calling the writer (self-heal).
func (o *keyContactWriterOrchestrator) normalizeAndValidateCreate(
	ctx context.Context,
	in *KeyContactCreateInput,
) (*model.KeyContact, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.LastName = strings.TrimSpace(in.LastName)

	if in.Email == "" {
		return nil, pkgerrors.NewValidation("email is required")
	}
	if in.FirstName == "" {
		return nil, pkgerrors.NewValidation("first_name is required")
	}
	if in.LastName == "" {
		return nil, pkgerrors.NewValidation("last_name is required")
	}

	existing, err := o.storage.ListKeyContactsForMembership(ctx, in.MembershipUID)
	if err != nil {
		return nil, err
	}

	activePerRole := make(map[string]int)
	for _, kc := range existing {
		if !strings.EqualFold(kc.Status, constants.RoleStatusInactive) {
			activePerRole[kc.Role]++
			// Self-heal: same role + same email already active → return as-is.
			// Short-circuit: writer will not be called so the capacity check below is moot.
			if kc.Role == in.Role && strings.EqualFold(kc.Email, in.Email) {
				return kc, nil
			}
		}
	}

	// Note: this count-based check has a TOCTOU window — two concurrent requests for the
	// same membership+role can both see count N-1 and both insert. The DUPLICATE_VALUE
	// self-heal in writer.go catches exact (Asset, Contact) duplicates but not a role
	// over-capacity race using different Contacts. Strict enforcement would require an
	// SF-side unique rule on (Asset__c, Role__c, Status__c='Active'); until then, rare
	// over-capacity records can be corrected via /admin/reindex or manual SF cleanup.
	if limit, ok := constants.KeyContactRoleLimits[in.Role]; ok && activePerRole[in.Role] >= limit {
		return nil, pkgerrors.NewConflict(
			fmt.Sprintf("%s is limited to %d per membership", in.Role, limit))
	}

	return nil, nil
}

// normalizeAndValidateUpdate lowercases email (if set), then enforces duplicate
// detection whenever email or role changes, and capacity only when role changes
// (legacy guard pattern: capacity re-applies only on a role change).
func (o *keyContactWriterOrchestrator) normalizeAndValidateUpdate(
	ctx context.Context,
	current *model.KeyContact,
	in *KeyContactUpdateInput,
) error {
	if in.Email != nil {
		*in.Email = strings.ToLower(strings.TrimSpace(*in.Email))
	}

	newRole := current.Role
	if in.Role != nil {
		newRole = *in.Role
	}

	roleChanging := newRole != current.Role
	emailChanging := in.Email != nil && !strings.EqualFold(*in.Email, current.Email)
	// Re-activating an inactive contact can exceed capacity or create a (role,email) collision.
	statusActivating := in.Status != nil &&
		strings.EqualFold(*in.Status, constants.RoleStatusActive) &&
		!strings.EqualFold(current.Status, constants.RoleStatusActive)

	// Nothing that affects uniqueness or capacity is changing — skip sibling scan.
	if !roleChanging && !emailChanging && !statusActivating {
		return nil
	}

	existing, err := o.storage.ListKeyContactsForMembership(ctx, current.MembershipUID)
	if err != nil {
		return err
	}

	effectiveEmail := current.Email
	if in.Email != nil {
		effectiveEmail = *in.Email
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
