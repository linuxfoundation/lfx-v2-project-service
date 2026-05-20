// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	sf "github.com/k-capehart/go-salesforce/v3"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// soqlAccountID is a minimal projection used when only the Salesforce Id is needed.
type soqlAccountID struct {
	ID string `salesforce:"Id" json:"Id"`
}

// accountsSOQLBase is the SELECT and fixed WHERE base for Account search/list
// queries. The caller appends optional LIKE predicates and an ORDER BY clause
// before executing. Only Accounts that are not deleted and have at least one
// membership Asset are returned — the SOQL semi-join on Asset ensures we do
// not expose arbitrary Account records to the API.
const accountsSOQLBase = `
SELECT
    Id, Name, Logo_URL__c, Website,
    Account_Domain__c, Domain_Alias__c,
    Description, Phone, ParentId,
    Parent.Id, Parent.Name, Parent.Logo_URL__c,
    Industry, Sector__c, CrunchBase_URL__c,
    NumberOfEmployees, LF_Membership_Status__c, IsMember__c,
    CreatedDate, LastModifiedDate
FROM Account
WHERE IsDeleted = false
    AND Id IN (
        SELECT AccountId FROM Asset
        WHERE Product2.Family = 'Membership'
            AND IsDeleted = false
    )`

// AccountRepo handles Salesforce SOQL queries for Account (B2BOrg) records.
type AccountRepo struct {
	client *sf.Salesforce
}

// NewAccountRepo creates a new AccountRepo backed by the given Salesforce client.
func NewAccountRepo(client *sf.Salesforce) *AccountRepo {
	return &AccountRepo{client: client}
}

// FetchChildUIDsByParentUID returns the v2 UUIDs of all Salesforce Accounts
// whose ParentId matches the SFID derived from parentUID. The result is used
// to build the FGA child-list tuples required for the b2b_org hierarchy cascade.
// Returns an empty slice (no error) when the parent has no children.
func (r *AccountRepo) FetchChildUIDsByParentUID(ctx context.Context, parentUID string) ([]string, error) {
	parentSFID, err := sfuuid.ToSFID(parentUID)
	if err != nil {
		return nil, fmt.Errorf("converting parent uid %q to sfid: %w", parentUID, err)
	}
	slog.DebugContext(ctx, "fetching child account SFIDs from Salesforce", "parent_sfid", parentSFID)

	query := "SELECT Id FROM Account WHERE ParentId = " + quoteSOQL(parentSFID) + " AND IsDeleted = false"
	records, _, err := QueryAllPages[soqlAccountID](ctx, r.client, query, "")
	if err != nil {
		return nil, fmt.Errorf("fetching children of parent %s: %w", parentSFID, err)
	}

	uids := make([]string, 0, len(records))
	for _, rec := range records {
		uid, convErr := sfuuid.ToUUID(rec.ID)
		if convErr != nil {
			slog.WarnContext(ctx, "child account SFID could not be converted to UUID, skipping",
				"parent_sfid", parentSFID, "child_sfid", rec.ID)
			continue
		}
		uids = append(uids, uid)
	}
	return uids, nil
}

// FetchAccountBySFID fetches a single Account record from Salesforce by its
// Salesforce Id. Returns nil, nil when no matching record is found.
func (r *AccountRepo) FetchAccountBySFID(ctx context.Context, sfid string) (*model.B2BOrg, error) {
	slog.DebugContext(ctx, "fetching account by SFID from Salesforce", "sfid", sfid)
	query := accountsSOQLBase + "\n    AND Id = " + quoteSOQL(sfid) + "\nLIMIT 1"
	sfResult, err := QueryPage[soqlAccount](ctx, r.client, query, "")
	if err != nil {
		return nil, fmt.Errorf("fetching account by SFID %s: %w", sfid, err)
	}
	if len(sfResult.Records) == 0 {
		return nil, nil
	}
	return convertSOQLToB2BOrg(ctx, sfResult.Records[0])
}

// convertSOQLToB2BOrg converts a Salesforce Account SOQL result to the domain
// B2BOrg model.
func convertSOQLToB2BOrg(ctx context.Context, acc soqlAccount) (*model.B2BOrg, error) {
	uid, err := sfuuid.ToUUID(acc.ID)
	if err != nil {
		return nil, fmt.Errorf("converting account SFID %q to UUID: %w", acc.ID, err)
	}

	org := &model.B2BOrg{
		UID:  uid,
		SFID: acc.ID,
		Name: acc.Name,
	}

	// Normalize Website: ensure it has a scheme so that url.Parse produces a
	// host. If the value cannot be parsed at all, omit it with a warning.
	if raw := derefString(acc.Website); raw != "" {
		if u, parseErr := url.Parse(raw); parseErr == nil {
			if u.Scheme == "" {
				u.Scheme = "http"
			}
			org.Website = u.String()
		} else {
			slog.WarnContext(ctx, "account website could not be parsed, omitting",
				"sfid", acc.ID,
				"raw_value", raw,
			)
		}
	}

	// Normalize Account_Domain__c into PrimaryDomain.
	if raw := derefString(acc.PrimaryDomain); raw != "" {
		if normalized, ok := normalizeDomain(raw); ok {
			org.PrimaryDomain = normalized
		} else {
			slog.WarnContext(ctx, "account primary domain does not look like a valid domain, omitting",
				"sfid", acc.ID,
				"raw_value", raw,
			)
		}
	}

	// Normalize Domain_Alias__c (comma-separated) into DomainAliases.
	if raw := derefString(acc.DomainAlias); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if normalized, ok := normalizeDomain(item); ok {
				org.DomainAliases = append(org.DomainAliases, normalized)
			} else {
				slog.WarnContext(ctx, "account domain alias item does not look like a valid domain, omitting",
					"sfid", acc.ID,
					"raw_value", item,
				)
			}
		}
	}

	org.LogoURL = derefString(acc.LogoURL)
	org.Description = derefString(acc.Description)
	org.Phone = derefString(acc.Phone)
	org.Industry = derefString(acc.Industry)
	org.Sector = derefString(acc.Sector)
	org.CrunchBaseURL = acc.CrunchBaseURL
	org.NumberOfEmployees = acc.NumberOfEmployees
	org.Status = derefString(acc.Status)
	if acc.IsMember != nil {
		org.IsMember = *acc.IsMember
	}

	if parentSFID := derefString(acc.ParentID); parentSFID != "" {
		parentUID, convErr := sfuuid.ToUUID(parentSFID)
		if convErr != nil {
			slog.WarnContext(ctx, "account parent SFID could not be converted to UUID, omitting",
				"sfid", acc.ID, "parent_sfid", parentSFID)
		} else {
			org.ParentUID = parentUID
			if acc.Parent != nil {
				org.ParentDetail = &model.B2BOrgParentDetail{
					UID:     parentUID,
					Name:    acc.Parent.Name,
					LogoURL: acc.Parent.LogoURL,
				}
			}
		}
	}

	org.CreatedAt = parseSOQLTime(acc.CreatedDate)
	org.UpdatedAt = parseSOQLTime(acc.LastModifiedDate)

	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = org.CreatedAt
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt = time.Now()
	}

	return org, nil
}

// normalizeDomain validates and returns the host portion of a bare domain
// string. Values containing "/" or " " are rejected immediately. Bare domains
// (no scheme) are handled by reading u.Path, which is where url.Parse places
// them when no scheme is present.
func normalizeDomain(s string) (string, bool) {
	if strings.ContainsAny(s, "/ ") {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	host := u.Host
	if host == "" {
		// Bare domain: url.Parse puts it in Path when there is no scheme.
		host = u.Path
	}
	if host == "" {
		return "", false
	}
	return host, true
}
