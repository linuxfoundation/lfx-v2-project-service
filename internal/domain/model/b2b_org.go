// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "time"

// B2BOrg represents a B2B organization in the LFX v2 domain. It is the
// canonical entity for a member company.
type B2BOrg struct {
	// UID is the invertible UUID v8 for this organization.
	UID string `json:"uid"`

	// SFID is the raw Salesforce Account.Id. It is kept internal (not
	// serialized) so it is not exposed in API responses.
	SFID string `json:"-"`

	// Name is the organization's display name.
	Name string `json:"name"`

	// Website is the organization's website URL. Always has a scheme (http or
	// https). Omitted when empty or unparseable.
	Website string `json:"website,omitempty"`

	// PrimaryDomain is the normalized primary domain for the organization.
	// Expected to be a bare host such as "example.com"; values that do not
	// parse as a valid domain are omitted. Omitted when empty or invalid.
	PrimaryDomain string `json:"primary_domain,omitempty"`

	// DomainAliases is the list of additional normalized domains for the
	// organization. Each item is normalized with the same rules as
	// PrimaryDomain; invalid items are dropped.
	DomainAliases []string `json:"domain_aliases,omitempty"`

	// LogoURL is the URL of the organization's logo image.
	LogoURL string `json:"logo_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// B2BOrgInput carries the mutable fields for creating or updating a B2BOrg
// record. All fields are optional on update; a zero value means "leave
// unchanged".
type B2BOrgInput struct {
	// Name is the organization's display name.
	Name string

	// Website is the organization's website link.
	Website string

	// PrimaryDomain is the canonical primary domain for the organization.
	PrimaryDomain string

	// LogoURL is the URL of the organization's logo image.
	LogoURL string
}
