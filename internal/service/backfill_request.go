// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// BackfillRequest carries the validated, normalised parameters for a single run.
type BackfillRequest struct {
	RunID  string
	Types  []string   // empty = all in-scope (full/since modes)
	Since  *time.Time // nil = full reindex
	Items  []ReindexItem
	DryRun bool
}

// ReindexItem identifies a single entity in targeted (items) mode.
type ReindexItem struct {
	Type string
	UID  string
}

// ValidateAndBuildRequest validates the payload and returns a BackfillRequest.
func ValidateAndBuildRequest(p *membershipservice.AdminReindexPayload) (BackfillRequest, error) {
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
