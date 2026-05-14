// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// KeyContactReader assembles a fully-denormalized model.KeyContact from its
// constituent Salesforce sObjects: Project_Role__c, Contact, Asset, and Account.
// Each sObject is fetched via the sObject REST API with conditional-GET caching
// in the NATS member-service-cache KV bucket.
type KeyContactReader struct {
	client *SObjectClient
}

// NewKeyContactReader returns a KeyContactReader backed by the given SObjectClient.
func NewKeyContactReader(client *SObjectClient) *KeyContactReader {
	return &KeyContactReader{client: client}
}

// AssembleKeyContact fetches all related sObjects for the given Project_Role__c UID
// and returns a fully-denormalized KeyContact. The returned time.Time is the
// oldest Last-Modified across the constituent records; callers use it as the
// ETag / If-Unmodified-Since value.
//
// Step 1 (sequential): Fetch Project_Role__c → extract AssetID, ContactID.
// Step 2 (parallel via errgroup): Fetch Contact (if ContactID is set) + Fetch Asset.
// Step 3 (parallel via errgroup): Fetch Account + Fetch Project__c (both depend on Asset).
func (r *KeyContactReader) AssembleKeyContact(ctx context.Context, uid string) (*model.KeyContact, time.Time, error) {
	// Step 1: Fetch the Project_Role__c record.
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, time.Time{}, errs.NewValidation(fmt.Sprintf("invalid Project_Role__c UID %q: %v", uid, err))
	}

	roleCacheKey := sobjectCacheKey(sobjectKeyPrefixKeyContact, uid)
	roleResult, err := r.client.FetchSObject(ctx, "Project_Role__c", sfid, roleCacheKey, projectRoleFields)
	if err != nil {
		return nil, time.Time{}, err
	}

	var rawRole sobjectProjectRole
	if err := json.Unmarshal(roleResult.Body, &rawRole); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal Project_Role__c sObject response: %w", err)
	}

	membershipUID, err := sfuuid.ToUUID(rawRole.AssetID)
	if err != nil && rawRole.AssetID != "" {
		return nil, time.Time{}, fmt.Errorf("convert Project_Role__c.Asset__c %q to UUID: %w", rawRole.AssetID, err)
	}

	contact := &model.KeyContact{
		UID:            uid,
		MembershipUID:  membershipUID,
		Role:           derefString(rawRole.Role),
		Status:         derefString(rawRole.Status),
		BoardMember:    rawRole.BoardMember,
		PrimaryContact: rawRole.PrimaryContact,
		CreatedAt:      parseSOQLTime(rawRole.CreatedDate),
		UpdatedAt:      parseSOQLTime(rawRole.LastModifiedDate),
	}

	roleLastMod := parseSOQLTime(rawRole.LastModifiedDate)
	oldestLastMod := roleLastMod

	// Step 2: Parallel fetch Contact + Asset.
	var (
		rawContact     sobjectContact
		rawAsset       sobjectAsset
		contactLastMod time.Time
		assetLastMod   time.Time
	)

	assetUID, assetUIDErr := sfuuid.ToUUID(rawRole.AssetID)
	if assetUIDErr != nil && rawRole.AssetID != "" {
		return nil, time.Time{}, fmt.Errorf("convert Project_Role__c.Asset__c to UUID: %w", assetUIDErr)
	}

	g, gCtx := errgroup.WithContext(ctx)

	if rawRole.ContactID != nil && *rawRole.ContactID != "" {
		contactSFID := *rawRole.ContactID
		g.Go(func() error {
			// Derive a stable cache key; fall back to raw SFID if UUID conversion fails.
			cacheKey := "contact." + contactSFID
			if contactUID, convErr := sfuuid.ToUUID(contactSFID); convErr == nil {
				cacheKey = sobjectCacheKey(sobjectKeyPrefixContact, contactUID)
			}

			result, fetchErr := r.client.FetchSObject(gCtx, "Contact", contactSFID, cacheKey, contactFields)
			if fetchErr != nil {
				return fetchErr
			}
			if unmarshalErr := json.Unmarshal(result.Body, &rawContact); unmarshalErr != nil {
				return fmt.Errorf("unmarshal Contact sObject response: %w", unmarshalErr)
			}
			contactLastMod = parseSOQLTime(rawContact.LastModifiedDate)
			return nil
		})
	}

	if rawRole.AssetID != "" {
		assetCacheKey := sobjectCacheKey(sobjectKeyPrefixProjectMembership, assetUID)
		g.Go(func() error {
			result, fetchErr := r.client.FetchSObject(gCtx, "Asset", rawRole.AssetID, assetCacheKey, assetFields)
			if fetchErr != nil {
				return fetchErr
			}
			if unmarshalErr := json.Unmarshal(result.Body, &rawAsset); unmarshalErr != nil {
				return fmt.Errorf("unmarshal Asset sObject response: %w", unmarshalErr)
			}
			assetLastMod = parseSOQLTime(rawAsset.LastModifiedDate)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, time.Time{}, err
	}

	// Populate contact personal fields from Contact.
	contact.FirstName = derefString(rawContact.FirstName)
	contact.LastName = derefString(rawContact.LastName)
	contact.Title = derefString(rawContact.Title)
	contact.Email = derefString(rawContact.Email)

	// Populate TierUID from Asset.Product2Id.
	if rawAsset.Product2ID != "" {
		if tierUID, tierErr := sfuuid.ToUUID(rawAsset.Product2ID); tierErr == nil {
			contact.TierUID = tierUID
		}
	}

	// Step 3: Parallel fetch Account + Project__c (both sourced from Asset fields).
	var (
		accountLastMod time.Time
		projLastMod    time.Time
	)

	g2, gCtx2 := errgroup.WithContext(ctx)

	if rawAsset.AccountID != "" {
		g2.Go(func() error {
			accountUID, convErr := sfuuid.ToUUID(rawAsset.AccountID)
			if convErr != nil {
				return fmt.Errorf("convert Asset.AccountId %q to UUID: %w", rawAsset.AccountID, convErr)
			}

			org, _, fetchErr := r.client.FetchB2BOrg(gCtx2, accountUID)
			if fetchErr != nil {
				return fetchErr
			}

			contact.CompanyName = org.Name
			contact.CompanyLogoURL = org.LogoURL
			// CompanyDomain matches the SOQL path which uses Account.Website.
			contact.CompanyDomain = org.Website
			contact.B2BOrgUID = accountUID
			accountLastMod = org.UpdatedAt
			return nil
		})
	}

	if rawAsset.ProjectsID != nil && *rawAsset.ProjectsID != "" {
		projectSFID := *rawAsset.ProjectsID
		g2.Go(func() error {
			projCacheKey := sobjectCacheKey(sobjectKeyPrefixProject, projectSFID)
			result, fetchErr := r.client.FetchSObject(gCtx2, "Project__c", projectSFID, projCacheKey, projectRecordFields)
			if fetchErr != nil {
				// Project resolution failure is non-fatal; log and continue.
				slog.WarnContext(ctx, "failed to fetch Project__c for key contact assembly",
					"contact_uid", uid,
					"project_sfid", projectSFID,
					"error", fetchErr,
				)
				return nil
			}
			var rawProj sobjectProjectRecord
			if unmarshalErr := json.Unmarshal(result.Body, &rawProj); unmarshalErr != nil {
				return fmt.Errorf("unmarshal Project__c sObject response: %w", unmarshalErr)
			}
			contact.ProjectSlug = derefString(rawProj.Slug)
			projLastMod = parseSOQLTime(rawProj.LastModifiedDate)
			return nil
		})
	}

	if err := g2.Wait(); err != nil {
		return nil, time.Time{}, err
	}

	// Compute the oldest Last-Modified across all constituent records.
	for _, t := range []time.Time{roleLastMod, contactLastMod, assetLastMod, accountLastMod, projLastMod} {
		if !t.IsZero() && (oldestLastMod.IsZero() || t.Before(oldestLastMod)) {
			oldestLastMod = t
		}
	}
	if oldestLastMod.IsZero() {
		oldestLastMod = time.Now()
	}

	return contact, oldestLastMod, nil
}
