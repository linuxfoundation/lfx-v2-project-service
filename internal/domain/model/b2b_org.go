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

// B2BOrgFilters holds the SOQL-pushable filter predicates for SearchB2BOrgs.
// Only non-empty fields are applied. The zero value means "no filtering".
type B2BOrgFilters struct {
	// NameSearch is a free-text substring to match against Account.Name via a
	// SOQL LIKE predicate. MUST always be lowercase — callers normalise with
	// strings.ToLower before setting this field so that the same value can be
	// used in both the SOQL query and the NATS KV cache key.
	NameSearch string

	// SortOrder controls the ORDER BY clause in the SOQL query. Defaults to
	// SortOrderName when not set.
	SortOrder SortOrder

	// PageToken is an opaque cursor returned in a previous B2BOrgPage response.
	// When non-empty it is decoded to continue fetching from the cache chain.
	PageToken string
}

// EffectiveSortOrder returns the sort order to apply, substituting the default
// (name ascending) when none is explicitly set.
func (f B2BOrgFilters) EffectiveSortOrder() SortOrder {
	if f.SortOrder == "" {
		return SortOrderName
	}
	return f.SortOrder
}

// B2BOrgPage is the result of a paginated SearchB2BOrgs call. It carries the
// current page of B2BOrg records and an opaque cursor token for the next page
// (empty when this is the last page).
type B2BOrgPage struct {
	// Orgs is the current page of B2BOrg records.
	Orgs []*B2BOrg

	// NextPageToken is an opaque cursor that can be passed as
	// B2BOrgFilters.PageToken to retrieve the next page. Empty string means
	// this is the last page.
	NextPageToken string

	// TotalSize is the total number of records matching the query as reported
	// by Salesforce. Set on the first page; may be 0 on subsequent pages.
	TotalSize int
}
