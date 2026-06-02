// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package model

import (
	"fmt"
	"time"
)

// ProjectMembership represents a membership (Asset) record scoped to a specific
// project. Account (company) attributes are denormalized directly onto this
// struct — there is no separate Account sub-object. This eliminates the
// fan-out problem of maintaining per-project permission tuples on a shared
// account object.
type ProjectMembership struct {
	// UID is the invertible UUID v8 derived from the Salesforce Asset.Id.
	UID string `json:"uid"`

	// TierUID is the UID of the associated MembershipTier (Product2).
	TierUID string `json:"tier_uid"`

	// ProjectUID is the v2 UUID of the project this membership belongs to.
	// Resolved from the project slug via project-service over NATS.
	ProjectUID string `json:"project_uid"`

	// ProjectSFID is the 18-char Salesforce Project__c.Id for this membership's
	// project. Populated directly from the SFDC record. Exposed in API responses
	// and indexer docs so Salesforce-keyed consumers can correlate the project.
	ProjectSFID string `json:"project_sfid,omitempty"`

	// ProjectSlug is the URL slug of the associated project (e.g. "kubernetes").
	// Populated from the Projects__r relationship on the Salesforce Asset record.
	// Included in API responses and preserved in the cache so that ProjectUID
	// can be resolved from it after a cache round-trip.
	ProjectSlug string `json:"project_slug,omitempty"`

	// AccountSFID is the raw Salesforce Account.Id for this membership's
	// company. Used internally by the write path to associate new Contact
	// records with the correct Account; not included in API responses.
	AccountSFID string `json:"-"`

	// B2BOrgUID is the canonical 18-char Salesforce Account.Id used as uid.
	// Not included in API responses until the B2BOrg entity is surfaced
	// through a dedicated endpoint.
	B2BOrgUID string `json:"b2b_org_uid,omitempty"`

	// Status is the membership status, e.g. "Active", "Expired".
	Status string `json:"status"`

	// Year is the membership year, e.g. "2025".
	Year string `json:"year,omitempty"`

	// Tier is the membership tier label, e.g. "Gold".
	Tier string `json:"tier,omitempty"`

	// AutoRenew indicates whether automatic renewal is enabled.
	AutoRenew bool `json:"auto_renew"`

	// RenewalType describes the renewal cadence, e.g. "Annual".
	RenewalType string `json:"renewal_type,omitempty"`

	// Price is the current membership price.
	Price float64 `json:"price,omitempty"`

	// AnnualFullPrice is the full annual list price before any discounts.
	AnnualFullPrice float64 `json:"annual_full_price,omitempty"`

	// PaymentFrequency describes how often payments are made.
	PaymentFrequency string `json:"payment_frequency,omitempty"`

	// PaymentTerms are the payment terms, e.g. "Net 30".
	PaymentTerms string `json:"payment_terms,omitempty"`

	// AgreementDate is the date the membership agreement was signed.
	AgreementDate string `json:"agreement_date,omitempty"`

	// PurchaseDate is the effective purchase date (COALESCE of PurchaseDate,
	// InstallDate, CreatedDate from the Asset record).
	PurchaseDate string `json:"purchase_date,omitempty"`

	// StartDate is the membership start date (InstallDate on the Asset).
	StartDate string `json:"start_date,omitempty"`

	// EndDate is the membership end date (UsageEndDate on the Asset).
	EndDate string `json:"end_date,omitempty"`

	// CompanyName is the name of the member company, denormalized from Account.
	CompanyName string `json:"company_name"`

	// CompanyLogoURL is the member company logo URL, denormalized from Account.
	CompanyLogoURL string `json:"company_logo_url,omitempty"`

	// CompanyDomain is the member company website/domain, denormalized from
	// Account.Website. Used for domain-based lookups (e.g. MCP).
	CompanyDomain string `json:"company_domain,omitempty"`

	// TierName is the product name denormalized from Product2, e.g. "Gold
	// Corporate Membership".
	TierName string `json:"tier_name,omitempty"`

	// TierFamily is the product family denormalized from Product2, e.g.
	// "Membership".
	TierFamily string `json:"tier_family,omitempty"`

	// TierProductType is the product type denormalized from Product2.
	TierProductType string `json:"tier_product_type,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tags returns search tags for this membership. The indexer uses these to make
// the record discoverable by UID and by parent relationships.
func (pm *ProjectMembership) Tags() []string {
	if pm == nil {
		return nil
	}
	var tags []string
	if pm.UID != "" {
		tags = append(tags, pm.UID)
		tags = append(tags, fmt.Sprintf("project_membership_uid:%s", pm.UID))
	}
	if pm.ProjectUID != "" {
		tags = append(tags, fmt.Sprintf("project_uid:%s", pm.ProjectUID))
	}
	if pm.ProjectSFID != "" {
		tags = append(tags, fmt.Sprintf("project_sfid:%s", pm.ProjectSFID))
	}
	if pm.B2BOrgUID != "" {
		tags = append(tags, fmt.Sprintf("b2b_org_uid:%s", pm.B2BOrgUID))
	}
	return tags
}
