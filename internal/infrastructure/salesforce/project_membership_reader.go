// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// sobjectProjectRecord is the JSON shape of a Salesforce Project__c sObject
// record used to populate project context on ProjectMembership and KeyContact.
type sobjectProjectRecord struct {
	ID               string  `json:"Id"`
	Name             string  `json:"Name"`
	Slug             *string `json:"Slug__c"`
	LogoURL          *string `json:"Project_Logo__c"`
	LastModifiedDate string  `json:"LastModifiedDate"`
}

// Project__c sObject field list for reads.
const (
	projectRecordFields     = "Id,Name,Slug__c,Project_Logo__c,LastModifiedDate"
	sobjectKeyPrefixProject = "project"
)

// ProjectMembershipReader implements port.ProjectMembershipReader using the
// Salesforce sObject REST API to assemble a fully denormalised ProjectMembership
// from its constituent objects (Asset, Account, Product2, Project__c).
type ProjectMembershipReader struct {
	client *SObjectClient
}

// NewProjectMembershipReader creates a ProjectMembershipReader backed by the
// given SObjectClient.
func NewProjectMembershipReader(client *SObjectClient) *ProjectMembershipReader {
	return &ProjectMembershipReader{client: client}
}

// Ensure ProjectMembershipReader satisfies the port at compile time.
var _ port.ProjectMembershipReader = (*ProjectMembershipReader)(nil)

// AssembleProjectMembership fetches all related sObjects for the given Asset UID
// and returns a fully denormalised ProjectMembership. The returned time.Time is
// the oldest Last-Modified across the constituent records.
//
// Step 1 (sequential): Fetch Asset by UID and unmarshal to model.ProjectMembership.
// Step 2 (parallel via errgroup): 3 goroutines:
//   - If AccountID is set: Fetch Account (B2BOrg) and populate CompanyName, Logo, Domain, B2BOrgUID
//   - If TierUID is set: Fetch Product2 and populate TierName, TierFamily, TierProductType
//   - If ProjectsID is set: Fetch Project__c and populate ProjectUID, ProjectSlug
//
// Step 3: Track the oldest LastModified timestamp across all fetches.
func (r *ProjectMembershipReader) AssembleProjectMembership(ctx context.Context, uid string) (*model.ProjectMembership, time.Time, error) {
	// Step 1: Fetch Asset and unmarshal to base ProjectMembership.
	sfid, err := sfuuid.Normalize18(uid)
	if err != nil {
		return nil, time.Time{}, errs.NewValidation(fmt.Sprintf("invalid Asset UID %q: %v", uid, err))
	}

	cacheKey := sobjectCacheKey(sobjectKeyPrefixProjectMembership, uid)
	result, err := r.client.FetchSObject(ctx, "Asset", sfid, cacheKey, assetFields)
	if err != nil {
		return nil, time.Time{}, err
	}

	var rawAsset sobjectAsset
	if unmarshalErr := json.Unmarshal(result.Body, &rawAsset); unmarshalErr != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal Asset sObject response: %w", unmarshalErr)
	}

	membership, err := sobjectAssetToModel(&rawAsset, sfid)
	if err != nil {
		return nil, time.Time{}, err
	}

	// Track the oldest timestamp; start with Asset's.
	assetLastMod := parseSOQLTime(rawAsset.LastModifiedDate)
	oldestLastMod := assetLastMod

	// Step 2: Parallel fetches for Account, Product2, and Project__c.
	var (
		b2bLastMod  time.Time
		tierLastMod time.Time
		projLastMod time.Time
	)

	g, gCtx := errgroup.WithContext(ctx)

	// Fetch Account (B2BOrg) if AccountID is present.
	if rawAsset.AccountID != "" {
		g.Go(func() error {
			accountUID, err := sfuuid.Normalize18(rawAsset.AccountID)
			if err != nil {
				return fmt.Errorf("normalize Asset.AccountId %q: %w", rawAsset.AccountID, err)
			}

			org, _, err := r.client.FetchB2BOrg(gCtx, accountUID)
			if err != nil {
				return err
			}

			// Populate company-related fields.
			membership.CompanyName = org.Name
			membership.CompanyLogoURL = org.LogoURL
			membership.CompanyDomain = org.Website
			membership.B2BOrgUID = accountUID
			membership.AccountSFID = rawAsset.AccountID

			// Track timestamp for oldest calculation.
			b2bLastMod = org.UpdatedAt

			return nil
		})
	}

	// Fetch Product2 (MembershipTier) if Product2ID is present.
	if membership.TierUID != "" {
		g.Go(func() error {
			tier, err := r.client.FetchProduct2(gCtx, membership.TierUID)
			if err != nil {
				return err
			}

			// Populate tier-related fields.
			membership.TierName = tier.Name
			membership.TierFamily = tier.Family
			membership.TierProductType = tier.ProductType

			// Track timestamp for oldest calculation.
			tierLastMod = tier.UpdatedAt

			return nil
		})
	}

	// Fetch Project__c if Projects__c is present.
	if rawAsset.ProjectsID != nil && *rawAsset.ProjectsID != "" {
		g.Go(func() error {
			projectSFID := *rawAsset.ProjectsID
			projectCacheKey := sobjectCacheKey(sobjectKeyPrefixProject, projectSFID)

			result, err := r.client.FetchSObject(gCtx, "Project__c", projectSFID, projectCacheKey, projectRecordFields)
			if err != nil {
				return err
			}

			var rawProj sobjectProjectRecord
			if unmarshalErr := json.Unmarshal(result.Body, &rawProj); unmarshalErr != nil {
				return fmt.Errorf("unmarshal Project__c sObject response: %w", unmarshalErr)
			}

			// Populate project-related fields.
			// ProjectUID is resolved from the slug via project-service (NATS); see
			// MemberReader and backfill_runner for the resolution step.
			// ProjectSFID is the raw Salesforce Project__c.Id.
			if rawProj.ID != "" {
				if projectSFID, normErr := sfuuid.Normalize18(rawProj.ID); normErr == nil {
					membership.ProjectSFID = projectSFID
				}
			}
			membership.ProjectSlug = derefString(rawProj.Slug)

			// Track timestamp for oldest calculation.
			projLastMod = parseSOQLTime(rawProj.LastModifiedDate)

			return nil
		})
	}

	// Wait for all parallel fetches to complete.
	if err := g.Wait(); err != nil {
		return nil, time.Time{}, err
	}

	// Step 3: Find the oldest timestamp among all fetches (skip zero values).
	candidates := []time.Time{assetLastMod, b2bLastMod, tierLastMod, projLastMod}
	for _, t := range candidates {
		if !t.IsZero() {
			if oldestLastMod.IsZero() || t.Before(oldestLastMod) {
				oldestLastMod = t
			}
		}
	}

	// Ensure UpdatedAt is set to at least the oldest timestamp from the constituent records.
	if oldestLastMod.IsZero() {
		oldestLastMod = time.Now()
	}
	if membership.UpdatedAt.IsZero() {
		membership.UpdatedAt = oldestLastMod
	}

	return membership, oldestLastMod, nil
}
