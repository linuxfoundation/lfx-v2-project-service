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

// accountsSOQLBase is the SELECT and fixed WHERE base for Account search/list
// queries. The caller appends optional LIKE predicates and an ORDER BY clause
// before executing. Only Accounts that are not deleted and have at least one
// membership Asset are returned — the SOQL semi-join on Asset ensures we do
// not expose arbitrary Account records to the API.
const accountsSOQLBase = `
SELECT
    Id, Name, Logo_URL__c, Website,
    Account_Domain__c, Domain_Alias__c,
    CreatedDate, LastModifiedDate
FROM Account
WHERE IsDeleted = false
    AND Id IN (
        SELECT AccountId FROM Asset
        WHERE Product2.Family = 'Membership'
            AND IsDeleted = false
    )`

// FirstAccountBatchResult is the return type of FetchFirstAccountBatch. It
// carries the first sfQueryBatchSize Account records, the Salesforce locator
// for the remainder (if any), and the total result set size reported by
// Salesforce.
type FirstAccountBatchResult struct {
	// Records is the full first SF batch (up to sfQueryBatchSize records).
	Records []*model.B2BOrg

	// SFLocator is the raw Salesforce nextRecordsUrl for the records beyond
	// the first batch. Empty when the first batch contains all results (i.e.
	// the result set is ≤ sfQueryBatchSize records).
	SFLocator string

	// TotalSize is the total record count reported by Salesforce.
	TotalSize int
}

// AccountRepo handles Salesforce SOQL queries for Account (B2BOrg) records.
type AccountRepo struct {
	client *sf.Salesforce
}

// NewAccountRepo creates a new AccountRepo backed by the given Salesforce client.
func NewAccountRepo(client *sf.Salesforce) *AccountRepo {
	return &AccountRepo{client: client}
}

// buildAccountsSOQL assembles the full SOQL query string for
// FetchFirstAccountBatch, appending an optional LIKE predicate for NameSearch
// and an ORDER BY clause. All interpolated values are passed through quoteSOQL
// to prevent injection.
func buildAccountsSOQL(_ context.Context, filters model.B2BOrgFilters) string {
	var b strings.Builder
	b.WriteString(accountsSOQLBase)
	if filters.NameSearch != "" {
		// NameSearch is always lowercase by contract (normalised by the
		// caller), so the same value is used in both the SOQL query and the
		// NATS KV cache key. quoteLikeSOQL handles escaping and quoting in a
		// single pass, producing a complete '%term%' literal for interpolation.
		fmt.Fprintf(&b, "\n    AND Name LIKE %s", quoteLikeSOQL(filters.NameSearch))
	}
	b.WriteString(accountSortOrderClause(filters.EffectiveSortOrder()))
	return b.String()
}

// accountSortOrderClause returns the ORDER BY fragment for Account queries.
// An unrecognised or empty sort order falls back to name ascending.
func accountSortOrderClause(order model.SortOrder) string {
	switch order {
	case model.SortOrderLastModified:
		return "\nORDER BY LastModifiedDate DESC NULLS LAST"
	case model.SortOrderNewest:
		return "\nORDER BY CreatedDate DESC NULLS LAST"
	default:
		// SortOrderName and any unrecognised value.
		return "\nORDER BY Name ASC NULLS LAST"
	}
}

// FetchFirstAccountBatch issues a single SOQL query for the first
// sfQueryBatchSize Account records matching the given filters, returning the
// full batch and the Salesforce locator for any remaining records. The caller
// is responsible for following the locator in a background goroutine via
// QueryAllPages if SFLocator is non-empty.
func (r *AccountRepo) FetchFirstAccountBatch(ctx context.Context, filters model.B2BOrgFilters) (FirstAccountBatchResult, error) {
	slog.DebugContext(ctx, "fetching first account batch from Salesforce",
		"name_search", filters.NameSearch,
		"sort_order", filters.EffectiveSortOrder(),
	)

	query := buildAccountsSOQL(ctx, filters)
	sfResult, err := QueryPage[soqlAccount](ctx, r.client, query, "")
	if err != nil {
		return FirstAccountBatchResult{}, fmt.Errorf("fetching first account batch: %w", err)
	}

	records := make([]*model.B2BOrg, 0, len(sfResult.Records))
	for _, acc := range sfResult.Records {
		org, convErr := convertSOQLToB2BOrg(ctx, acc)
		if convErr != nil {
			slog.WarnContext(ctx, "skipping account with invalid SFID",
				"sfid", acc.ID,
				"error", convErr,
			)
			continue
		}
		records = append(records, org)
	}

	return FirstAccountBatchResult{
		Records:   records,
		SFLocator: sfResult.NextPageToken,
		TotalSize: sfResult.TotalSize,
	}, nil
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
