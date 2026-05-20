// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

const (
	RoleNameRepresentativeVotingContact = "Representative/Voting Contact"
	RoleNameAuthorizedSignatory         = "Authorized Signatory"
	RoleNameBillingContact              = "Billing Contact"
	RoleNameMarketingContact            = "Marketing Contact"
	RoleNameTechnicalContact            = "Technical Contact"
	RoleNameLegalContact                = "Legal Contact"
	RoleNameEventSponsorshipContact     = "Event Sponsorship Contact"
	RoleNamePOContact                   = "PO Contact"
	RoleNamePRContact                   = "PR Contact"

	RoleStatusActive   = "Active"
	RoleStatusInactive = "Inactive"
)

// KeyContactRoles is the canonical ordered list of valid Project_Role__c role values.
var KeyContactRoles = []any{
	RoleNameRepresentativeVotingContact,
	RoleNameAuthorizedSignatory,
	RoleNameBillingContact,
	RoleNameMarketingContact,
	RoleNameTechnicalContact,
	RoleNameLegalContact,
	RoleNameEventSponsorshipContact,
	RoleNamePOContact,
	RoleNamePRContact,
}

// KeyContactStatuses is the canonical list of valid Project_Role__c status values.
var KeyContactStatuses = []any{RoleStatusActive, RoleStatusInactive}

// KeyContactRoleLimits maps each role name to the maximum number of active contacts
// allowed per membership. Roles absent from this map are uncapped.
var KeyContactRoleLimits = map[string]int{
	RoleNameRepresentativeVotingContact: 1,
	RoleNameAuthorizedSignatory:         1,
	RoleNameBillingContact:              3,
	RoleNameLegalContact:                3,
	RoleNameEventSponsorshipContact:     3,
	RoleNamePOContact:                   3,
	RoleNamePRContact:                   3,
	RoleNameMarketingContact:            10,
	RoleNameTechnicalContact:            10,
}
