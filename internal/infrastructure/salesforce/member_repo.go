// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	sf "github.com/k-capehart/go-salesforce/v3"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// soqlProduct2 represents a Salesforce Product2 record returned by a SOQL query.
// The Project__c relationship is followed via Project__r to get project metadata.
type soqlProduct2 struct {
	ID               string            `salesforce:"Id"`
	Name             string            `salesforce:"Name"`
	Family           *string           `salesforce:"Family"`
	Type             *string           `salesforce:"Type__c"`
	ProjectID        *string           `salesforce:"Project__c"`
	IsDeleted        bool              `salesforce:"IsDeleted"`
	CreatedDate      string            `salesforce:"CreatedDate"`
	LastModifiedDate string            `salesforce:"LastModifiedDate"`
	Project          *soqlAssetProject `salesforce:"Project__r"`
}

// tierByIDSOQL fetches a single Product2 record by its Salesforce ID.
// The caller must substitute a quoteSOQL-escaped ID for the %s placeholder.
const tierByIDSOQL = `
SELECT
    Id, Name, Family, Type__c, Project__c,
    CreatedDate, LastModifiedDate,
    Project__r.Id, Project__r.Name, Project__r.Slug__c, Project__r.Status__c
FROM Product2
WHERE Id = %s
    AND Family = 'Membership'
    AND IsDeleted = false
`

// tiersByProjectSOQL fetches all Product2 (membership tier) records for a given
// Salesforce Project__c ID. The caller must substitute a quoteSOQL-escaped ID
// for the %s placeholder.
const tiersByProjectSOQL = `
SELECT
    Id, Name, Family, Type__c, Project__c,
    CreatedDate, LastModifiedDate,
    Project__r.Id, Project__r.Name, Project__r.Slug__c, Project__r.Status__c
FROM Product2
WHERE Project__c = %s
    AND Family = 'Membership'
    AND IsDeleted = false
`

// MemberRepo handles Salesforce SOQL queries for membership tiers (Product2
// records). Each tier is scoped to a single Project__c.
type MemberRepo struct {
	client *sf.Salesforce
}

// NewMemberRepo creates a new MemberRepo backed by the given Salesforce client.
func NewMemberRepo(client *sf.Salesforce) *MemberRepo {
	return &MemberRepo{client: client}
}

// FetchTierBySFID fetches a single membership Product2 record by its Salesforce
// ID. Returns nil if the record is not found or is not a membership product.
func (r *MemberRepo) FetchTierBySFID(ctx context.Context, sfid string) (*model.MembershipTier, error) {
	slog.DebugContext(ctx, "fetching membership tier from Salesforce by SFID", "sfid", sfid)

	var products []soqlProduct2
	if err := r.client.Query(fmt.Sprintf(tierByIDSOQL, quoteSOQL(sfid)), &products); err != nil {
		return nil, fmt.Errorf("fetching membership tier by SFID %s: %w", sfid, err)
	}

	if len(products) == 0 {
		return nil, nil
	}

	return convertSOQLToMembershipTier(products[0])
}

// FetchTiersByProjectSFID fetches all membership Product2 records for a given
// Salesforce Project__c ID, returning them as MembershipTier domain objects.
func (r *MemberRepo) FetchTiersByProjectSFID(ctx context.Context, projectSFID string) ([]*model.MembershipTier, error) {
	slog.DebugContext(ctx, "fetching membership tiers for project from Salesforce",
		"project_sfid", projectSFID,
	)

	var products []soqlProduct2
	if err := r.client.Query(fmt.Sprintf(tiersByProjectSOQL, quoteSOQL(projectSFID)), &products); err != nil {
		return nil, fmt.Errorf("fetching membership tiers for project %s: %w", projectSFID, err)
	}

	tiers := make([]*model.MembershipTier, 0, len(products))
	for _, p := range products {
		t, err := convertSOQLToMembershipTier(p)
		if err != nil {
			slog.WarnContext(ctx, "skipping membership tier with invalid SFID",
				"sfid", p.ID,
				"error", err,
			)
			continue
		}
		tiers = append(tiers, t)
	}

	return tiers, nil
}

// convertSOQLToMembershipTier converts a Salesforce Product2 SOQL result to the
// domain MembershipTier model. Project fields are denormalized from the
// Project__r relationship when available.
func convertSOQLToMembershipTier(p soqlProduct2) (*model.MembershipTier, error) {
	uid, err := sfuuid.Normalize18(p.ID)
	if err != nil {
		return nil, fmt.Errorf("normalizing product2 SFID %q: %w", p.ID, err)
	}

	t := &model.MembershipTier{
		UID:         uid,
		Name:        p.Name,
		Family:      derefString(p.Family),
		ProductType: derefString(p.Type),
	}

	// Populate ProjectSlug (and ProjectUID when available) from the Project__r
	// relationship. Both fields are now decoded correctly via salesforce tags.
	if p.Project != nil {
		t.ProjectSlug = derefString(p.Project.Slug)
	}

	// Timestamps.
	t.CreatedAt = parseSOQLTime(p.CreatedDate)
	t.UpdatedAt = parseSOQLTime(p.LastModifiedDate)

	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	return t, nil
}
