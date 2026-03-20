// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package model contains the domain model types for the LFX v2 member service.
// Models are structured around three project-scoped types that reflect the
// drill-down API layout: MembershipTier (product per project), ProjectMembership
// (asset per project), and ProjectKeyContact (project role per membership).
package model

import "time"

// MembershipTier represents a membership product (Product2) offered under a
// specific project. It is the parent type for ProjectMembership records — each
// membership belongs to exactly one tier within a project.
type MembershipTier struct {
	// UID is the invertible UUID v8 derived from the Salesforce Product2.Id.
	UID string `json:"uid"`

	// ProjectUID is the v2 UUID of the project this tier belongs to.
	ProjectUID string `json:"project_uid"`

	// ProjectSlug is the URL slug of the associated project. Used internally
	// by the resolver to populate ProjectUID; not included in API responses.
	ProjectSlug string `json:"-"`

	// Name is the product name, e.g. "Gold Corporate Membership".
	Name string `json:"name"`

	// Family is the product family, e.g. "Membership".
	Family string `json:"family,omitempty"`

	// ProductType is the product type field (Type__c on Product2).
	ProductType string `json:"product_type,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
