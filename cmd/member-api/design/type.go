// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
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
	dsl.Attribute("total_size", dsl.Int, "Total number of records matching the query. Set on the first page; may be 0 on continuation pages.", func() {
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

// IfMatchAttribute is the DSL attribute for the If-Match request header (conditional request).
func IfMatchAttribute() {
	dsl.Attribute("if_match", dsl.String, "If-Match header value for conditional requests", func() {
		dsl.Example("123")
	})
}

// IfNoneMatchAttribute is the DSL attribute for the If-None-Match request header (conditional request).
func IfNoneMatchAttribute() {
	dsl.Attribute("if_none_match", dsl.String, "If-None-Match header value for conditional requests", func() {
		dsl.Example("123")
	})
}

// IfModifiedSinceAttribute is the DSL attribute for the If-Modified-Since request header (conditional request).
func IfModifiedSinceAttribute() {
	dsl.Attribute("if_modified_since", dsl.String, "If-Modified-Since header value for conditional requests (HTTP date format)", func() {
		dsl.Example("Wed, 21 Oct 2025 07:28:00 GMT")
	})
}

// IfUnmodifiedSinceAttribute is the DSL attribute for the If-Unmodified-Since request header (conditional request).
func IfUnmodifiedSinceAttribute() {
	dsl.Attribute("if_unmodified_since", dsl.String, "If-Unmodified-Since header value for conditional requests (HTTP date format)", func() {
		dsl.Example("Wed, 21 Oct 2025 07:28:00 GMT")
	})
}

// LastModifiedAttribute is the DSL attribute for the Last-Modified response header.
func LastModifiedAttribute() {
	dsl.Attribute("last_modified", dsl.String, "Last-Modified header value (HTTP date format)", func() {
		dsl.Example("Wed, 21 Oct 2025 07:28:00 GMT")
	})
}

// B2BOrgUIDAttribute adds the b2b_org_uid path parameter attribute.
func B2BOrgUIDAttribute() {
	dsl.Attribute("b2b_org_uid", dsl.String, "B2BOrg UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
}

// B2BOrgResponse is the DSL type for a B2B organization response.
var B2BOrgResponse = dsl.Type("b2b-org-response", func() {
	dsl.Description("A B2B organization")
	dsl.Attribute("uid", dsl.String, "B2BOrg UID (invertible UUID v8)", func() {
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
		dsl.Format(dsl.FormatUUID)
	})
	dsl.Attribute("name", dsl.String, "Organization name", func() {
		dsl.Example("Example Corp")
	})
	dsl.Attribute("description", dsl.String, "Organization free-text description (Account.Description)", func() {
		dsl.Example("A leading technology company")
	})
	dsl.Attribute("phone", dsl.String, "Organization contact phone number (Account.Phone)", func() {
		dsl.Example("+1-555-000-0000")
	})
	dsl.Attribute("website", dsl.String, "Organization website URL; always has a scheme (http or https)", func() {
		dsl.Format(dsl.FormatURI)
		dsl.Example("https://example.com")
	})
	dsl.Attribute("primary_domain", dsl.String, "Primary domain; bare host only, no scheme or path, e.g. 'example.com'", func() {
		dsl.Example("example.com")
	})
	dsl.Attribute("domain_aliases", dsl.ArrayOf(dsl.String), "Additional domains; each item is a bare host with the same normalization as primary_domain", func() {
		dsl.Example([]string{"example.org", "example.net"})
	})
	dsl.Attribute("logo_url", dsl.String, "URL of the organization logo (Account.Logo_URL__c)", func() {
		dsl.Example("https://example.com/logo.png")
	})
	dsl.Attribute("industry", dsl.String, "Industry classification (Account.Industry, standard Salesforce field)", func() {
		dsl.Example("Technology")
	})
	dsl.Attribute("sector", dsl.String, "Sector classification (Account.Sector__c, custom Salesforce field)", func() {
		dsl.Example("Software")
	})
	dsl.Attribute("crunch_base_url", dsl.String, "CrunchBase profile URL (Account.CrunchBase_URL__c)", func() {
		dsl.Example("https://www.crunchbase.com/organization/example-corp")
	})
	dsl.Attribute("number_of_employees", dsl.Int, "Employee count (Account.NumberOfEmployees)", func() {
		dsl.Example(500)
	})
	dsl.Attribute("status", dsl.String, "LF membership status (Account.LF_Membership_Status__c); read-only, managed by Salesforce workflows", func() {
		dsl.Example("Active")
	})
	dsl.Attribute("is_member", dsl.Boolean, "Whether the organization is currently an LF member (Account.IsMember__c); read-only, managed by Salesforce workflows", func() {
		dsl.Example(true)
	})
	// TODO: slug is reserved for Account.Slug__c once the field is confirmed to exist in the SF org schema.
	dsl.Attribute("slug", dsl.String, "URL-friendly organization identifier; populated when Account.Slug__c is available", func() {
		dsl.Example("example-corp")
	})
	dsl.Attribute("parent_uid", dsl.String, "UID of the parent organization (Account.ParentId); omitted when no parent", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("5c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
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

// B2BOrgCreateBody is the DSL type for creating a B2B organization.
// POST /b2b_orgs registers an Account that EasyCLA / LFX Enrollment has already
// created in Salesforce; it does not create a new Account. Idempotent on sfid.
var B2BOrgCreateBody = dsl.Type("b2b-org-create-body", func() {
	dsl.Description("Request body for registering a B2B organization by its Salesforce Account ID")
	dsl.Attribute("sfid", dsl.String, "Salesforce Account.Id (15- or 18-character); used to fetch and cache the org record", func() {
		dsl.Example("001Hs00001AbCdEFAZ")
		dsl.MinLength(15)
		dsl.MaxLength(18)
	})
	dsl.Required("sfid")
})

// B2BOrgUpdateBody is the DSL type for updating a B2B organization.
var B2BOrgUpdateBody = dsl.Type("b2b-org-update-body", func() {
	dsl.Description("Request body for updating mutable fields on a B2B organization")
	dsl.Attribute("name", dsl.String, "Organization name", func() {
		dsl.Example("Example Corp")
	})
	dsl.Attribute("description", dsl.String, "Organization free-text description", func() {
		dsl.Example("A leading technology company")
	})
	dsl.Attribute("phone", dsl.String, "Organization contact phone number", func() {
		dsl.Example("+1-555-000-0000")
	})
	dsl.Attribute("website", dsl.String, "Organization website URL", func() {
		dsl.Example("https://example.com")
	})
	dsl.Attribute("primary_domain", dsl.String, "Primary domain (bare host)", func() {
		dsl.Example("example.com")
	})
	dsl.Attribute("logo_url", dsl.String, "URL of the organization logo (Account.Logo_URL__c)", func() {
		dsl.Example("https://example.com/logo.png")
	})
	dsl.Attribute("industry", dsl.String, "Industry classification (Account.Industry)", func() {
		dsl.Example("Technology")
	})
	dsl.Attribute("sector", dsl.String, "Sector classification (Account.Sector__c)", func() {
		dsl.Example("Software")
	})
	dsl.Attribute("crunch_base_url", dsl.String, "CrunchBase profile URL (Account.CrunchBase_URL__c); pass empty string to explicitly clear", func() {
		dsl.Example("https://www.crunchbase.com/organization/example-corp")
	})
	dsl.Attribute("number_of_employees", dsl.Int, "Employee count (Account.NumberOfEmployees)", func() {
		dsl.Example(500)
	})
})

// KeyContactCreateBody is the DSL type for creating a key contact.
var KeyContactCreateBody = dsl.Type("key-contact-create-body", func() {
	dsl.Description("Request body for creating a key contact (Project_Role__c record)")
	dsl.Attribute("b2b_org_uid", dsl.String, "UID of the B2B organization (Account)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("project_uid", dsl.String, "V2 project UUID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("a27394a3-7a6c-4d0f-9e0f-692d8753924f")
	})
	dsl.Attribute("membership_uid", dsl.String, "Membership UID", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Attribute("email", dsl.String, "Contact email address; used to resolve or create the Salesforce Contact record", func() {
		dsl.Format(dsl.FormatEmail)
		dsl.Example("john.doe@example.com")
	})
	dsl.Attribute("first_name", dsl.String, "Contact first name; used when creating a new Contact on miss", func() {
		dsl.Example("John")
	})
	dsl.Attribute("last_name", dsl.String, "Contact last name; used when creating a new Contact on miss", func() {
		dsl.Example("Doe")
	})
	dsl.Attribute("title", dsl.String, "Contact job title; used when creating a new Contact on miss", func() {
		dsl.Example("CTO")
	})
	dsl.Attribute("role", dsl.String, "Contact role designation, e.g. 'Voting Representative'", func() {
		dsl.Example("Voting Representative")
		dsl.Enum(constants.KeyContactRoles...)
	})
	dsl.Attribute("status", dsl.String, "Role record status, e.g. 'Active'", func() {
		dsl.Example("Active")
		dsl.Enum(constants.KeyContactStatuses...)
	})
	dsl.Attribute("board_member", dsl.Boolean, "Whether this contact holds a board member role", func() {
		dsl.Example(false)
	})
	dsl.Attribute("primary_contact", dsl.Boolean, "Whether this is the primary contact for the membership", func() {
		dsl.Example(false)
	})
	dsl.Required("b2b_org_uid", "project_uid", "membership_uid", "email", "first_name", "last_name", "role")
})

// KeyContactUpdateBody is the DSL type for updating a key contact.
var KeyContactUpdateBody = dsl.Type("key-contact-update-body", func() {
	dsl.Description("Request body for updating a key contact (Project_Role__c record)")
	dsl.Attribute("email", dsl.String, "Contact email address; normalized to lowercase before update", func() {
		dsl.Format(dsl.FormatEmail)
		dsl.Example("john.doe@example.com")
	})
	dsl.Attribute("role", dsl.String, "Contact role designation, e.g. 'Voting Representative'", func() {
		dsl.Example("Voting Representative")
		dsl.Enum(constants.KeyContactRoles...)
	})
	dsl.Attribute("status", dsl.String, "Role record status, e.g. 'Active'", func() {
		dsl.Example("Active")
		dsl.Enum(constants.KeyContactStatuses...)
	})
	dsl.Attribute("board_member", dsl.Boolean, "Whether this contact holds a board member role", func() {
		dsl.Example(false)
	})
	dsl.Attribute("primary_contact", dsl.Boolean, "Whether this is the primary contact for the membership", func() {
		dsl.Example(false)
	})
	dsl.Attribute("title", dsl.String, "Contact job title", func() {
		dsl.Example("CTO")
	})
})

// AdminReindexPayload is the DSL type for the reindex admin action request.
var AdminReindexPayload = dsl.Type("admin-reindex-payload", func() {
	dsl.Description("Request payload for triggering a reindex operation")
	dsl.Attribute("types", dsl.ArrayOf(dsl.String), "List of entity types to reindex (optional; if empty, reindex all types)", func() {
		dsl.Example([]string{"membership", "key_contact"})
	})
})

// AdminReindexResult is the DSL type for the reindex admin action result.
var AdminReindexResult = dsl.Type("admin-reindex-result", func() {
	dsl.Description("Result of a reindex operation")
	dsl.Attribute("run_id", dsl.String, "Correlation ID for the reindex run (for log lookups)", func() {
		dsl.Format(dsl.FormatUUID)
		dsl.Example("4c46585f-9f01-8bda-a0a5-f0c8eeef7fff")
	})
	dsl.Required("run_id")
})
