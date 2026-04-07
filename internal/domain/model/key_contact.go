// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import "time"

// KeyContactInput carries the mutable fields for creating or updating a
// Project_Role__c key contact record in Salesforce. All pointer fields are
// optional on update; a nil pointer means "leave unchanged".
type KeyContactInput struct {
	// Email is the contact's email address. Required on create; used to
	// resolve or create the B2B Salesforce Contact record.
	Email string

	// FirstName is the contact's first name. Required on create; used when
	// creating a new Contact on miss.
	FirstName string

	// LastName is the contact's last name. Required on create; used when
	// creating a new Contact on miss.
	LastName string

	// Title is the contact's job title. Optional; used when creating a new
	// Contact on miss.
	Title string

	// MembershipUID is the UID of the parent membership (Asset). Required on
	// create; used for cache invalidation on update/delete.
	MembershipUID string

	// ProjectUID is the v2 UUID of the project this key contact belongs to.
	// When non-empty it is stamped onto the returned contact record so that
	// callers receive a fully-populated project_uid without an additional
	// resolver round-trip.
	ProjectUID string

	// AccountSFID is the Salesforce Account.Id of the membership's company.
	// Optional on create; when non-empty it is set as the Contact's AccountId
	// so the new Contact is associated with the correct company in Salesforce.
	AccountSFID string

	// Role is the contact's role designation, e.g. "Voting Representative".
	Role *string

	// Status is the role record status, e.g. "Active".
	Status *string

	// BoardMember indicates whether this contact holds a board member role.
	BoardMember *bool

	// PrimaryContact indicates whether this is the primary contact for the
	// membership.
	PrimaryContact *bool
}

// KeyContact represents a key contact (Project_Role__c) for a specific
// membership within a project. Contact and company attributes are denormalized
// directly onto this struct — there are no separate Contact or Organization
// sub-objects. This avoids any dependency on the User Service or Org Service.
type KeyContact struct {
	// UID is the invertible UUID v8 derived from the Salesforce Project_Role__c.Id.
	UID string `json:"uid"`

	// MembershipUID is the UID of the associated ProjectMembership (Asset).
	MembershipUID string `json:"membership_uid"`

	// TierUID is the UID of the associated MembershipTier (Product2).
	TierUID string `json:"tier_uid"`

	// ProjectUID is the v2 UUID of the project this key contact belongs to.
	ProjectUID string `json:"project_uid"`

	// ProjectSlug is the URL slug of the associated project. Used internally
	// by the resolver to populate ProjectUID; not included in API responses.
	ProjectSlug string `json:"-"`

	// B2BOrgUID is the invertible UUID v8 derived from the Salesforce
	// Account.Id of the membership's company. Populated from AccountId on the
	// parent Asset's Account relationship; not included in API responses until
	// the B2BOrg entity is surfaced through a dedicated endpoint.
	B2BOrgUID string `json:"b2b_org_uid,omitempty"`

	// Role is the contact's role designation, e.g. "Voting Representative".
	Role string `json:"role"`

	// Status is the role record status, e.g. "Active".
	Status string `json:"status"`

	// BoardMember indicates whether this contact holds a board member role.
	BoardMember bool `json:"board_member"`

	// PrimaryContact indicates whether this is the primary contact for the
	// membership.
	PrimaryContact bool `json:"primary_contact"`

	// FirstName is the contact's first name, denormalized from Contact.
	FirstName string `json:"first_name"`

	// LastName is the contact's last name, denormalized from Contact.
	LastName string `json:"last_name"`

	// Title is the contact's job title, denormalized from Contact.
	Title string `json:"title,omitempty"`

	// Email is the contact's primary email address, resolved from
	// Alternate_Email__c where Primary_Email__c = true. Used for email-based
	// lookups (e.g. MCP). No User Service reference is made.
	Email string `json:"email,omitempty"`

	// CompanyName is the member company name, denormalized from the Account
	// associated with the membership Asset. No Org Service reference is made.
	CompanyName string `json:"company_name"`

	// CompanyLogoURL is the member company logo URL, denormalized from Account.
	CompanyLogoURL string `json:"company_logo_url,omitempty"`

	// CompanyDomain is the member company website/domain, denormalized from
	// Account.Website. Used for domain-based lookups (e.g. MCP).
	CompanyDomain string `json:"company_domain,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
