// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "time"

// B2BOrg represents a B2B organization (Salesforce Account) in the LFX v2
// domain. It is the canonical entity for a member company, decoupled from the
// Salesforce wire format.
type B2BOrg struct {
	// UID is the invertible UUID v8 derived from the Salesforce Account.Id.
	UID string `json:"uid"`

	// SFID is the raw Salesforce Account.Id. It is kept internal (not
	// serialized) so it is not exposed in API responses.
	SFID string `json:"-"`

	// Name is the organization's display name.
	Name string `json:"name"`

	// Domain is the organization's primary website domain, e.g. "example.com".
	Domain string `json:"domain,omitempty"`

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

	// Domain is the organization's primary website domain.
	Domain string

	// LogoURL is the URL of the organization's logo image.
	LogoURL string
}
