// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// ─── Cache key prefix constants ───────────────────────────────────────────────

// sObject cache key prefix constants for the member-service-cache bucket.
// Keys follow the pattern "{prefix}.{uid}" as required by the architecture.
const (
	sobjectKeyPrefixB2BOrg            = "b2b_org"
	sobjectKeyPrefixProjectMembership = "project_membership"
	sobjectKeyPrefixKeyContact        = "key_contact"
	sobjectKeyPrefixMembershipTier    = "membership_tier"
)

// sobjectCacheKey returns the "{prefix}.{uid}" cache key for the
// member-service-cache bucket.
func sobjectCacheKey(prefix, uid string) string {
	return prefix + "." + uid
}

// ─── sObject REST API JSON types ──────────────────────────────────────────────

// These types mirror the Salesforce sObject REST API JSON field names. They are
// distinct from the soql* types (which use `salesforce:""` struct tags for the
// SOQL library) because the sObject REST endpoint returns standard JSON keys.

// sobjectAccount is the JSON shape of a Salesforce Account record from the
// sObject REST API. Used for B2BOrg (member company) lookups.
type sobjectAccount struct {
	ID               string  `json:"Id"`
	Name             string  `json:"Name"`
	LogoURL          *string `json:"Logo_URL__c"`
	Website          *string `json:"Website"`
	CreatedDate      string  `json:"CreatedDate"`
	LastModifiedDate string  `json:"LastModifiedDate"`
	SystemModstamp   string  `json:"SystemModstamp"`
}

// sobjectAsset is the JSON shape of a Salesforce Asset record from the sObject
// REST API. Used for ProjectMembership (membership / Asset) single-record reads.
type sobjectAsset struct {
	ID               string  `json:"Id"`
	Name             string  `json:"Name"`
	Status           *string `json:"Status"`
	AccountID        string  `json:"AccountId"`
	Product2ID       string  `json:"Product2Id"`
	Year             *string `json:"Year__c"`
	Tier             *string `json:"Tier__c"`
	RecordTypeID     *string `json:"RecordTypeId"`
	AutoRenew        bool    `json:"Auto_Renew__c"`
	RenewalType      *string `json:"Renewal_Type__c"`
	Price            float64 `json:"Price"`
	AnnualFullPrice  float64 `json:"Annual_Full_Price__c"`
	PaymentFrequency *string `json:"PaymentFrequency__c"`
	PaymentTerms     *string `json:"PaymentTerms__c"`
	AgreementDate    *string `json:"Agreement_Date__c"`
	PurchaseDate     *string `json:"PurchaseDate"`
	InstallDate      *string `json:"InstallDate"`
	UsageEndDate     *string `json:"UsageEndDate"`
	ProjectsID       *string `json:"Projects__c"`
	CreatedDate      string  `json:"CreatedDate"`
	LastModifiedDate string  `json:"LastModifiedDate"`
	SystemModstamp   string  `json:"SystemModstamp"`
}

// sobjectProduct2 is the JSON shape of a Salesforce Product2 record from the
// sObject REST API. Used for MembershipTier single-record reads.
type sobjectProduct2 struct {
	ID               string  `json:"Id"`
	Name             string  `json:"Name"`
	Family           *string `json:"Family"`
	Type             *string `json:"Type__c"`
	ProjectID        *string `json:"Project__c"`
	CreatedDate      string  `json:"CreatedDate"`
	LastModifiedDate string  `json:"LastModifiedDate"`
	SystemModstamp   string  `json:"SystemModstamp"`
}

// sobjectProjectRole is the JSON shape of a Salesforce Project_Role__c record
// from the sObject REST API. Used for KeyContact single-record reads.
type sobjectProjectRole struct {
	ID             string  `json:"Id"`
	AssetID        string  `json:"Asset__c"`
	ContactID      *string `json:"Contact__c"`
	Role           *string `json:"Role__c"`
	Status         *string `json:"Status__c"`
	BoardMember    bool    `json:"BoardMember__c"`
	PrimaryContact bool    `json:"PrimaryContact__c"`
	CreatedDate    string  `json:"CreatedDate"`
	SystemModstamp string  `json:"SystemModstamp"`
}

// ─── sObject field lists ──────────────────────────────────────────────────────

// Field lists sent in the ?fields= query parameter for each sObject type.
// Using explicit field selection avoids fetching unneeded fields and keeps
// responses small.
const (
	accountFields = "Id,Name,Logo_URL__c,Website,CreatedDate,LastModifiedDate,SystemModstamp"

	assetFields = "Id,Name,Status,AccountId,Product2Id,Year__c,Tier__c,RecordTypeId," +
		"Auto_Renew__c,Renewal_Type__c,Price,Annual_Full_Price__c,PaymentFrequency__c," +
		"PaymentTerms__c,Agreement_Date__c,PurchaseDate,InstallDate,UsageEndDate," +
		"Projects__c,CreatedDate,LastModifiedDate,SystemModstamp"

	product2Fields = "Id,Name,Family,Type__c,Project__c,CreatedDate,LastModifiedDate,SystemModstamp"

	projectRoleFields = "Id,Asset__c,Contact__c,Role__c,Status__c,BoardMember__c," +
		"PrimaryContact__c,CreatedDate,SystemModstamp"
)

// ─── FetchAccount ─────────────────────────────────────────────────────────────

// AccountRecord is a flat representation of a Salesforce Account record as
// returned by the sObject REST API. It carries only the fields used by the
// member service (name, logo, website). Relationship fields (e.g. Contacts,
// Assets) are not included — those require SOQL joins.
type AccountRecord struct {
	UID       string
	Name      string
	LogoURL   string
	Domain    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FetchAccount fetches a single Salesforce Account (B2BOrg) record by its UID.
// The SFID is derived from the UID; the cache key is "b2b_org.{uid}".
//
// Because the Account sObject has no natural project association in the returned
// JSON (relationship fields require SOQL sub-selects), the returned AccountRecord
// carries only the flat Account fields.
func (c *SObjectClient) FetchAccount(ctx context.Context, uid string) (*AccountRecord, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, errs.NewValidation(fmt.Sprintf("invalid Account UID %q: %v", uid, err))
	}

	cacheKey := sobjectCacheKey(sobjectKeyPrefixB2BOrg, uid)
	result, err := c.FetchSObject(ctx, "Account", sfid, cacheKey, accountFields)
	if err != nil {
		return nil, err
	}

	var raw sobjectAccount
	if unmarshalErr := json.Unmarshal(result.Body, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal Account sObject response: %w", unmarshalErr)
	}

	return sobjectAccountToRecord(&raw, uid), nil
}

// sobjectAccountToRecord converts a raw sobjectAccount to an AccountRecord.
func sobjectAccountToRecord(raw *sobjectAccount, uid string) *AccountRecord {
	return &AccountRecord{
		UID:       uid,
		Name:      raw.Name,
		LogoURL:   derefString(raw.LogoURL),
		Domain:    derefString(raw.Website),
		CreatedAt: parseSOQLTime(raw.CreatedDate),
		UpdatedAt: parseSOQLTime(raw.LastModifiedDate),
	}
}

// ─── FetchAsset ───────────────────────────────────────────────────────────────

// FetchAsset fetches a single Salesforce Asset (ProjectMembership) record by
// its UID using the sObject REST API. The returned model carries only the flat
// Asset fields; relationship fields (Account name/logo, Product2 name, Project
// slug) are not populated — callers that need those should use the SOQL-backed
// MemberReader instead.
//
// This method is intended for cache-validation use cases where a caller already
// has the denormalized data and only needs to check whether the record has
// changed.
func (c *SObjectClient) FetchAsset(ctx context.Context, uid string) (*model.ProjectMembership, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, errs.NewValidation(fmt.Sprintf("invalid Asset UID %q: %v", uid, err))
	}

	cacheKey := sobjectCacheKey(sobjectKeyPrefixProjectMembership, uid)
	result, err := c.FetchSObject(ctx, "Asset", sfid, cacheKey, assetFields)
	if err != nil {
		return nil, err
	}

	var raw sobjectAsset
	if unmarshalErr := json.Unmarshal(result.Body, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal Asset sObject response: %w", unmarshalErr)
	}

	return sobjectAssetToModel(&raw, uid)
}

// sobjectAssetToModel converts a raw sobjectAsset to a minimal
// model.ProjectMembership. Relationship-sourced fields (CompanyName, TierName,
// ProjectUID, etc.) are left at their zero values; the caller is responsible
// for enriching the record if needed.
func sobjectAssetToModel(raw *sobjectAsset, uid string) (*model.ProjectMembership, error) {
	tierUID, err := sfuuid.ToUUID(raw.Product2ID)
	if err != nil && raw.Product2ID != "" {
		return nil, fmt.Errorf("convert Asset.Product2Id %q to UUID: %w", raw.Product2ID, err)
	}

	purchaseDate := coalesceDate(raw.PurchaseDate, raw.InstallDate, &raw.CreatedDate)

	return &model.ProjectMembership{
		UID:              uid,
		TierUID:          tierUID,
		Status:           derefString(raw.Status),
		Year:             derefString(raw.Year),
		Tier:             derefString(raw.Tier),
		AutoRenew:        raw.AutoRenew,
		RenewalType:      derefString(raw.RenewalType),
		Price:            raw.Price,
		AnnualFullPrice:  raw.AnnualFullPrice,
		PaymentFrequency: derefString(raw.PaymentFrequency),
		PaymentTerms:     derefString(raw.PaymentTerms),
		AgreementDate:    parseSOQLDateTime(raw.AgreementDate),
		PurchaseDate:     parseSOQLDateTime(&purchaseDate),
		StartDate:        parseSOQLDateTime(raw.InstallDate),
		EndDate:          parseSOQLDateTime(raw.UsageEndDate),
		CreatedAt:        parseSOQLTime(raw.CreatedDate),
		UpdatedAt:        parseSOQLTime(raw.LastModifiedDate),
	}, nil
}

// coalesceDate returns the first non-nil, non-empty date string from the
// supplied pointers. Returns empty string when all are nil or empty.
func coalesceDate(candidates ...*string) string {
	for _, s := range candidates {
		if s != nil && *s != "" {
			return *s
		}
	}
	return ""
}

// ─── FetchProduct2 ────────────────────────────────────────────────────────────

// FetchProduct2 fetches a single Salesforce Product2 (MembershipTier) record by
// its UID using the sObject REST API. ProjectUID is not populated because the
// Product2 sObject has no project relationship field accessible without a SOQL
// join.
func (c *SObjectClient) FetchProduct2(ctx context.Context, uid string) (*model.MembershipTier, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, errs.NewValidation(fmt.Sprintf("invalid Product2 UID %q: %v", uid, err))
	}

	cacheKey := sobjectCacheKey(sobjectKeyPrefixMembershipTier, uid)
	result, err := c.FetchSObject(ctx, "Product2", sfid, cacheKey, product2Fields)
	if err != nil {
		return nil, err
	}

	var raw sobjectProduct2
	if unmarshalErr := json.Unmarshal(result.Body, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal Product2 sObject response: %w", unmarshalErr)
	}

	return sobjectProduct2ToModel(&raw, uid), nil
}

// sobjectProduct2ToModel converts a raw sobjectProduct2 to a model.MembershipTier.
// ProjectUID is left empty; callers that need it should enrich via the resolver.
func sobjectProduct2ToModel(raw *sobjectProduct2, uid string) *model.MembershipTier {
	return &model.MembershipTier{
		UID:         uid,
		Name:        raw.Name,
		Family:      derefString(raw.Family),
		ProductType: derefString(raw.Type),
		CreatedAt:   parseSOQLTime(raw.CreatedDate),
		UpdatedAt:   parseSOQLTime(raw.LastModifiedDate),
	}
}

// ─── FetchProjectRole ─────────────────────────────────────────────────────────

// FetchProjectRole fetches a single Salesforce Project_Role__c (KeyContact)
// record by its UID using the sObject REST API. Contact and Account fields
// (name, email, company) are not populated because they require SOQL joins;
// the returned model carries only the Project_Role__c record's own fields.
func (c *SObjectClient) FetchProjectRole(ctx context.Context, uid string) (*model.KeyContact, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, errs.NewValidation(fmt.Sprintf("invalid Project_Role__c UID %q: %v", uid, err))
	}

	cacheKey := sobjectCacheKey(sobjectKeyPrefixKeyContact, uid)
	result, err := c.FetchSObject(ctx, "Project_Role__c", sfid, cacheKey, projectRoleFields)
	if err != nil {
		return nil, err
	}

	var raw sobjectProjectRole
	if unmarshalErr := json.Unmarshal(result.Body, &raw); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal Project_Role__c sObject response: %w", unmarshalErr)
	}

	return sobjectProjectRoleToModel(&raw, uid)
}

// sobjectProjectRoleToModel converts a raw sobjectProjectRole to a minimal
// model.KeyContact. Contact-sourced fields (FirstName, LastName, Email,
// Title, CompanyName, etc.) are left at their zero values.
func sobjectProjectRoleToModel(raw *sobjectProjectRole, uid string) (*model.KeyContact, error) {
	membershipUID, err := sfuuid.ToUUID(raw.AssetID)
	if err != nil && raw.AssetID != "" {
		return nil, fmt.Errorf("convert Project_Role__c.Asset__c %q to UUID: %w", raw.AssetID, err)
	}

	return &model.KeyContact{
		UID:            uid,
		MembershipUID:  membershipUID,
		Role:           derefString(raw.Role),
		Status:         derefString(raw.Status),
		BoardMember:    raw.BoardMember,
		PrimaryContact: raw.PrimaryContact,
		CreatedAt:      parseSOQLTime(raw.CreatedDate),
		UpdatedAt:      parseSOQLTime(raw.SystemModstamp),
	}, nil
}
