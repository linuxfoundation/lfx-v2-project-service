// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	"goa.design/goa/v3/dsl"
)

// ── Response types ────────────────────────────────────────────────────────────

// MembershipTierResponse is the DSL type for a membership tier (Product2) response.
var MembershipTierResponse = dsl.Type("membership-tier-response", func() {
	dsl.Description("A membership tier (Product2) scoped to a project")
	dsl.Attribute("uid", dsl.String, "Tier UID (invertible UUID v8 from Product2.Id)", func() {
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
		dsl.Format(dsl.FormatUUID)
	})
	dsl.Attribute("project_uid", dsl.String, "V2 project UUID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("a27394a3-7a6c-4d0f-9e0f-692d8753924f")
	})
	dsl.Attribute("name", dsl.String, "Product name, e.g. 'Gold Corporate Membership'", func() {
		dsl.Example("Gold Corporate Membership")
	})
	dsl.Attribute("family", dsl.String, "Product family, e.g. 'Membership'", func() {
		dsl.Example("Membership")
	})
	dsl.Attribute("product_type", dsl.String, "Product type (Type__c)", func() {
		dsl.Example("Corporate")
	})
	dsl.Attribute("created_at", dsl.String, "Creation timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-01-01T00:00:00Z")
	})
	dsl.Attribute("updated_at", dsl.String, "Last update timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-06-01T12:00:00Z")
	})
})

// ProjectMembershipResponse is the DSL type for a project membership (Asset) response.
// Account (company) attributes are denormalized directly onto this type.
var ProjectMembershipResponse = dsl.Type("project-membership-response", func() {
	dsl.Description("A membership (Asset) scoped to a project, with denormalized company attributes")
	dsl.Attribute("uid", dsl.String, "Membership UID (invertible UUID v8 from Asset.Id)", func() {
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
		dsl.Format(dsl.FormatUUID)
	})
	dsl.Attribute("tier_uid", dsl.String, "UID of the associated membership tier (Product2)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("project_uid", dsl.String, "V2 project UUID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("a27394a3-7a6c-4d0f-9e0f-692d8753924f")
	})
	dsl.Attribute("project_slug", dsl.String, "URL slug of the project this membership belongs to", func() {
		dsl.Example("kubernetes")
	})
	dsl.Attribute("b2b_org_uid", dsl.String, "UID of the B2B organization (Account) this membership belongs to", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("status", dsl.String, "Membership status", func() {
		dsl.Example("Active")
	})
	dsl.Attribute("year", dsl.String, "Membership year", func() {
		dsl.Example("2025")
	})
	dsl.Attribute("tier", dsl.String, "Membership tier label", func() {
		dsl.Example("Gold")
	})
	dsl.Attribute("membership_type", dsl.String, "Membership type (derived from Asset RecordType)", func() {
		dsl.Example("Corporate")
	})
	dsl.Attribute("auto_renew", dsl.Boolean, "Whether automatic renewal is enabled", func() {
		dsl.Example(true)
	})
	dsl.Attribute("renewal_type", dsl.String, "Renewal cadence", func() {
		dsl.Example("Annual")
	})
	dsl.Attribute("price", dsl.Float64, "Current membership price", func() {
		dsl.Example(50000.0)
	})
	dsl.Attribute("annual_full_price", dsl.Float64, "Full annual list price before discounts", func() {
		dsl.Example(50000.0)
	})
	dsl.Attribute("payment_frequency", dsl.String, "Payment frequency", func() {
		dsl.Example("Annual")
	})
	dsl.Attribute("payment_terms", dsl.String, "Payment terms", func() {
		dsl.Example("Net 30")
	})
	dsl.Attribute("agreement_date", dsl.String, "Date the membership agreement was signed", func() {
		dsl.Example("2025-01-01T00:00:00Z")
	})
	dsl.Attribute("purchase_date", dsl.String, "Effective purchase date", func() {
		dsl.Example("2025-01-15T00:00:00Z")
	})
	dsl.Attribute("start_date", dsl.String, "Membership start date", func() {
		dsl.Example("2025-02-01T00:00:00Z")
	})
	dsl.Attribute("end_date", dsl.String, "Membership end date", func() {
		dsl.Example("2025-12-31T23:59:59Z")
	})
	// Denormalized company (Account) fields.
	dsl.Attribute("company_name", dsl.String, "Member company name (denormalized from Account)", func() {
		dsl.Example("Example Corp")
	})
	dsl.Attribute("company_logo_url", dsl.String, "Member company logo URL (denormalized from Account)", func() {
		dsl.Example("https://example.com/logo.png")
	})
	dsl.Attribute("company_domain", dsl.String, "Member company website/domain (denormalized from Account.Website)", func() {
		dsl.Example("https://example.com")
	})
	// Denormalized tier (Product2) fields.
	dsl.Attribute("tier_name", dsl.String, "Product name (denormalized from Product2)", func() {
		dsl.Example("Gold Corporate Membership")
	})
	dsl.Attribute("tier_family", dsl.String, "Product family (denormalized from Product2)", func() {
		dsl.Example("Membership")
	})
	dsl.Attribute("tier_product_type", dsl.String, "Product type (denormalized from Product2)", func() {
		dsl.Example("Corporate")
	})
	dsl.Attribute("created_at", dsl.String, "Creation timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-01-01T00:00:00Z")
	})
	dsl.Attribute("updated_at", dsl.String, "Last update timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-06-01T12:00:00Z")
	})
})

// ProjectKeyContactResponse is the DSL type for a key contact (Project_Role__c) response.
// Contact and company attributes are denormalized directly onto this type — no
// sub-objects, no User Service or Org Service references.
var ProjectKeyContactResponse = dsl.Type("project-key-contact-response", func() {
	dsl.Description("A key contact (Project_Role__c) scoped to a membership, with denormalized contact and company attributes")
	dsl.Attribute("uid", dsl.String, "Key contact UID (invertible UUID v8 from Project_Role__c.Id)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("membership_uid", dsl.String, "UID of the associated membership (Asset)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("tier_uid", dsl.String, "UID of the associated membership tier (Product2)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("project_uid", dsl.String, "V2 project UUID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("a27394a3-7a6c-4d0f-9e0f-692d8753924f")
	})
	dsl.Attribute("b2b_org_uid", dsl.String, "UID of the B2B organization (Account) this key contact's membership belongs to", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("role", dsl.String, "Contact role designation", func() {
		dsl.Example("Voting Representative")
	})
	dsl.Attribute("status", dsl.String, "Role record status", func() {
		dsl.Example("Active")
	})
	dsl.Attribute("board_member", dsl.Boolean, "Whether this contact holds a board member role", func() {
		dsl.Example(false)
	})
	dsl.Attribute("primary_contact", dsl.Boolean, "Whether this is the primary contact for the membership", func() {
		dsl.Example(true)
	})
	// Denormalized Contact fields.
	dsl.Attribute("first_name", dsl.String, "Contact first name (denormalized from Contact)", func() {
		dsl.Example("John")
	})
	dsl.Attribute("last_name", dsl.String, "Contact last name (denormalized from Contact)", func() {
		dsl.Example("Doe")
	})
	dsl.Attribute("title", dsl.String, "Contact job title (denormalized from Contact)", func() {
		dsl.Example("CTO")
	})
	dsl.Attribute("email", dsl.String, "Primary email address from Alternate_Email__c where Primary_Email__c = true", func() {
		dsl.Example("john.doe@example.com")
	})
	// Denormalized company (Account via Asset) fields.
	dsl.Attribute("company_name", dsl.String, "Member company name (denormalized from Asset.Account)", func() {
		dsl.Example("Example Corp")
	})
	dsl.Attribute("company_logo_url", dsl.String, "Member company logo URL (denormalized from Asset.Account)", func() {
		dsl.Example("https://example.com/logo.png")
	})
	dsl.Attribute("company_domain", dsl.String, "Member company website/domain (denormalized from Asset.Account.Website)", func() {
		dsl.Example("https://example.com")
	})
	dsl.Attribute("created_at", dsl.String, "Creation timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-01-01T00:00:00Z")
	})
	dsl.Attribute("updated_at", dsl.String, "Last update timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-06-01T12:00:00Z")
	})
})

// ListMetadata is the DSL type for list pagination metadata.
var ListMetadata = dsl.Type("list-metadata", func() {
	dsl.Description("Pagination metadata for list responses")
	dsl.Attribute("total_size", dsl.Int, "Total number of records matching the query, as reported by Salesforce. Set on the first page; may be 0 on continuation pages.", func() {
		dsl.Example(100)
	})
	dsl.Attribute("next_page_token", dsl.String, "Opaque cursor for the next page. Pass this value as the page_token query parameter to retrieve the next page. Empty or absent when this is the last page.", func() {
		dsl.Example("")
	})
})

// ── Payload attribute helpers ─────────────────────────────────────────────────

// BearerTokenAttribute is the DSL attribute for the JWT bearer token.
func BearerTokenAttribute() {
	dsl.Token("bearer_token", dsl.String, func() {
		dsl.Description("JWT token issued by Heimdall")
		dsl.Example("eyJhbGci...")
	})
}

// VersionAttribute is the DSL attribute for the API version query parameter.
func VersionAttribute() {
	dsl.Attribute("version", dsl.String, "Version of the API", func() {
		dsl.Example("1")
		dsl.Enum("1")
	})
}

// ETagAttribute is the DSL attribute for the ETag response header.
func ETagAttribute() {
	dsl.Attribute("etag", dsl.String, "ETag header value", func() {
		dsl.Example("123")
	})
}

// ProjectUIDAttribute is the DSL attribute for the project path parameter.
// Callers pass a v2 project UUID.
func ProjectUIDAttribute() {
	dsl.Attribute("project_uid", dsl.String, "V2 project UUID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("a27394a3-7a6c-4d0f-9e0f-692d8753924f")
	})
}

// TierUIDAttribute is the DSL attribute for the tier (Product2) UID path parameter.
func TierUIDAttribute() {
	dsl.Attribute("tier_uid", dsl.String, "Membership tier UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// MembershipUIDAttribute is the DSL attribute for the membership (Asset) UID path parameter.
func MembershipUIDAttribute() {
	dsl.Attribute("membership_uid", dsl.String, "Membership UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// ContactUIDAttribute is the DSL attribute for the key contact UID path parameter.
func ContactUIDAttribute() {
	dsl.Attribute("contact_uid", dsl.String, "Key contact UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// PageSizeAttribute is the DSL attribute for the page-size query parameter.
// The server normalises the value to the nearest supported batch size (10, 50,
// 100, 200, 500, or 1000). Values below 10 are raised to 10; values above 1000
// are capped at 1000. The default of 200 matches the Salesforce minimum batch
// size and is appropriate for most callers.
func PageSizeAttribute() {
	dsl.Attribute("pageSize", dsl.Int, "Logical page size (1–1000). The server rounds up to the nearest supported size: 10, 50, 100, 200 (default), 500, or 1000. Sub-200 values fetch a 200-record Salesforce batch and slice client-side.", func() {
		dsl.Default(200)
		dsl.Minimum(1)
		dsl.Maximum(1000)
		dsl.Example(200)
	})
}

// PageTokenAttribute is the DSL attribute for the opaque continuation cursor.
// Consumers copy the next_page_token value from a previous list response
// metadata object into this parameter to fetch the next page. Tokens are issued
// by Salesforce and are valid for 15 minutes.
func PageTokenAttribute() {
	dsl.Attribute("pageToken", dsl.String, "Opaque continuation cursor returned in a previous list response metadata.next_page_token. Omit (or pass empty) to start from the first page. Valid for 15 minutes.", func() {
		dsl.Example("")
	})
}

// SortAttribute is the DSL attribute for the sort-order query parameter.
// Supported values: "name" (A→Z by company name), "newest" (CreatedDate DESC),
// "last_modified" (LastModifiedDate DESC). Defaults to "newest".
func SortAttribute() {
	dsl.Attribute("sort", dsl.String, "Sort order for results. One of: name (A→Z by company name), newest (default, CreatedDate DESC), last_modified (LastModifiedDate DESC).", func() {
		dsl.Default("newest")
		dsl.Enum("name", "newest", "last_modified")
		dsl.Example("newest")
	})
}

// FilterAttribute is the DSL attribute for the filter query parameter.
// Supported keys for membership list endpoints:
//
//   - tier_uid — Tier UUID (from ListProjectTiers). Decoded to a Salesforce
//     Product2Id for an exact-match SOQL filter. Only active
//     members are returned regardless of this filter.
//
// Note: status is not an exposed filter — all membership queries return active
// members only. Other keys (company_name, tier_name, project_slug) are applied
// in-process after the Salesforce query and are less efficient for large projects.
func FilterAttribute() {
	dsl.Attribute("filter", dsl.String, "Semicolon-separated key=value filter pairs. Supported: tier_uid (UUID from ListProjectTiers). All results are restricted to active members.", func() {
		dsl.Example("tier_uid=4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// SearchNameAttribute is the DSL attribute for searching memberships by company name.
func SearchNameAttribute() {
	dsl.Attribute("search_name", dsl.String, "Search memberships by member company name (case-insensitive substring match)", func() {
		dsl.Example("Linux")
	})
}

// B2BOrgSearchNameAttribute is the DSL attribute for searching B2B orgs by name.
func B2BOrgSearchNameAttribute() {
	dsl.Attribute("search_name", dsl.String, "Search organizations by name (case-insensitive substring match)", func() {
		dsl.Example("Linux")
	})
}

// B2BOrgUIDAttribute adds the b2b_org_uid path parameter attribute.
func B2BOrgUIDAttribute() {
	dsl.Attribute("b2b_org_uid", dsl.String, "B2BOrg UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// B2BOrgResponse is the DSL type for a B2B organization (Salesforce Account) response.
var B2BOrgResponse = dsl.Type("b2b-org-response", func() {
	dsl.Description("A B2B organization (Salesforce Account)")
	dsl.Attribute("uid", dsl.String, "B2BOrg UID (invertible UUID v8 from Account.Id)", func() {
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
		dsl.Format(dsl.FormatUUID)
	})
	dsl.Attribute("name", dsl.String, "Organization name", func() {
		dsl.Example("Example Corp")
	})
	dsl.Attribute("domain", dsl.String, "Organization website domain", func() {
		dsl.Example("https://example.com")
	})
	dsl.Attribute("logo_url", dsl.String, "URL of the organization logo", func() {
		dsl.Example("https://example.com/logo.png")
	})
	dsl.Attribute("created_at", dsl.String, "Creation timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-01-01T00:00:00Z")
	})
	dsl.Attribute("updated_at", dsl.String, "Last update timestamp", func() {
		dsl.Format(dsl.FormatDateTime)
		dsl.Example("2025-06-01T12:00:00Z")
	})
})
