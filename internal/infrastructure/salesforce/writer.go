// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package salesforce provides a Salesforce SOQL-backed implementation of
// port.MemberReader. Salesforce is the source of truth; NATS KV is used as a
// per-record TTL cache in front of it. Cache misses are transparent to callers.
package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sf "github.com/k-capehart/go-salesforce/v3"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// entityIsLockedCode is the Salesforce error code returned when a record is
// locked by another transaction (e.g. an approval process or trigger).
const entityIsLockedCode = "ENTITY_IS_LOCKED"

// writerMaxRetries is the maximum number of attempts made when a Salesforce
// mutation fails due to ENTITY_IS_LOCKED.
const writerMaxRetries = 3

// writerRetryDelay is the base wait time between lock-contention retries.
const writerRetryDelay = 500 * time.Millisecond

// sfProjectRole is the Salesforce DML struct for Project_Role__c. Fields use
// salesforce tags matching the Salesforce API field names. Id is required for
// update and delete; omitempty keeps it absent on insert so Salesforce auto-assigns it.
type sfProjectRole struct {
	ID             string  `salesforce:"Id,omitempty"`
	AssetID        string  `salesforce:"Asset__c,omitempty"`
	ContactID      string  `salesforce:"Contact__c,omitempty"`
	Role           *string `salesforce:"Role__c,omitempty"`
	Status         *string `salesforce:"Status__c,omitempty"`
	BoardMember    *bool   `salesforce:"BoardMember__c,omitempty"`
	PrimaryContact *bool   `salesforce:"PrimaryContact__c,omitempty"`
}

// sfProjectRoleDelete is a minimal struct used for Salesforce delete operations.
// Only the Id field is required.
type sfProjectRoleDelete struct {
	ID string `salesforce:"Id"`
}

// KeyContactWriter writes Project_Role__c records to Salesforce and invalidates
// the relevant NATS KV cache entries after each successful mutation.
type KeyContactWriter struct {
	client          *sf.Salesforce
	sobjectClient   *SObjectClient
	contacts        *KeyContactRepo
	contactResolver *ContactRepo
	cache           *nats.Storage
}

// NewKeyContactWriter creates a new KeyContactWriter backed by the given
// Salesforce client, sObject client (for conditional writes), key contact read
// repo (for post-write re-fetch), contact resolver (for email → Contact SFID
// resolution on create), and NATS KV cache (for invalidation).
func NewKeyContactWriter(
	client *sf.Salesforce,
	sobjectClient *SObjectClient,
	contacts *KeyContactRepo,
	contactResolver *ContactRepo,
	cache *nats.Storage,
) *KeyContactWriter {
	return &KeyContactWriter{
		client:          client,
		sobjectClient:   sobjectClient,
		contacts:        contacts,
		contactResolver: contactResolver,
		cache:           cache,
	}
}

// CreateKeyContact creates a new Project_Role__c record in Salesforce. The
// input must include a valid Email, FirstName, LastName, and MembershipUID. The
// Contact SFID is resolved (or a new Contact is created) via the contact
// resolver before the Project_Role__c insert. After a successful insert the
// key-contacts KV cache entry for the parent membership is deleted so the next
// read fetches fresh data. The newly created record is re-fetched from
// Salesforce and returned.
func (w *KeyContactWriter) CreateKeyContact(ctx context.Context, input model.KeyContactInput) (*model.KeyContact, error) {
	assetSFID, err := sfuuid.ToSFID(input.MembershipUID)
	if err != nil {
		// Treat a value that does not decode as an LFX_ UUID as a raw SFID.
		assetSFID = input.MembershipUID
	}

	if input.Email == nil {
		return nil, fmt.Errorf("email is required for CreateKeyContact")
	}

	// Convert B2BOrgUID (v2 UUID) to Salesforce Account.Id for new-Contact creation.
	// Empty string is fine for existing-contact resolution (accountSFID is only used on insert).
	accountSFID, _ := sfuuid.ToSFID(input.AccountSFID)

	contactSFID, created, err := w.contactResolver.ResolveOrCreateContact(
		ctx,
		*input.Email,
		input.FirstName,
		input.LastName,
		input.Title,
		accountSFID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolving contact for email %q: %w", *input.Email, err)
	}
	if created {
		slog.InfoContext(ctx, "new Salesforce Contact created during key contact creation",
			"email", input.Email,
			"contact_sfid", contactSFID,
		)
	}

	record := sfProjectRole{
		AssetID:        assetSFID,
		ContactID:      contactSFID,
		Role:           input.Role,
		Status:         input.Status,
		BoardMember:    input.BoardMember,
		PrimaryContact: input.PrimaryContact,
	}

	var result sf.SalesforceResult
	if err := retryOnLock(ctx, writerMaxRetries, writerRetryDelay, func() error {
		var insertErr error
		result, insertErr = w.client.InsertOne("Project_Role__c", record)
		return insertErr
	}); err != nil {
		// Check for DUPLICATE_VALUE or DUPLICATES_DETECTED errors and attempt self-heal.
		if strings.Contains(err.Error(), "DUPLICATE_VALUE") || strings.Contains(err.Error(), "DUPLICATES_DETECTED") {
			siblings, lookupErr := w.contacts.FetchKeyContactsByAssetSFID(ctx, assetSFID)
			if lookupErr == nil {
				// Find matching duplicate by email, role, and active status.
				for _, kc := range siblings {
					if kc.Status != constants.RoleStatusInactive && strings.EqualFold(kc.Email, *input.Email) &&
						input.Role != nil && kc.Role == *input.Role {
						// Self-heal: return the existing record.
						slog.InfoContext(ctx, "self-healed duplicate key contact",
							"asset_sfid", assetSFID,
							"email", *input.Email,
							"role", *input.Role,
							"existing_uid", kc.UID,
						)
						if input.ProjectUID != "" {
							kc.ProjectUID = input.ProjectUID
						}
						return kc, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("creating key contact in Salesforce: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("creating key contact in Salesforce: insert reported failure for new id %s", result.Id)
	}

	slog.DebugContext(ctx, "key contact created in Salesforce",
		"new_sfid", result.Id,
		"asset_sfid", assetSFID,
	)

	w.invalidateKeyContactsCache(ctx, input.MembershipUID)

	contact, err := w.contacts.FetchKeyContactBySFID(ctx, result.Id)
	if err != nil {
		return nil, fmt.Errorf("re-fetching created key contact %s: %w", result.Id, err)
	}
	if contact == nil {
		return nil, fmt.Errorf("re-fetching created key contact %s: record not found after insert", result.Id)
	}

	// Stamp ProjectUID from the input so the caller receives a fully-populated
	// response without requiring an additional resolver round-trip.
	if input.ProjectUID != "" {
		contact.ProjectUID = input.ProjectUID
	}

	return contact, nil
}

// UpdateKeyContact updates the mutable fields of an existing Project_Role__c
// record identified by contactUID. Only non-nil pointer fields in input are
// sent to Salesforce; nil fields are omitted from the patch so the existing
// values are preserved. If-Unmodified-Since from input is forwarded to Salesforce
// for server-side concurrency protection; Salesforce returns 412 when the record
// has been modified since that timestamp. If-Match (ETag) validation is handled in
// the service layer before this method is called — it is never forwarded to SF.
//
// After a successful update the key-contacts cache entry for the parent membership
// is deleted and the updated record is re-fetched.
func (w *KeyContactWriter) UpdateKeyContact(ctx context.Context, contactUID string, input model.KeyContactInput) (*model.KeyContact, error) {
	sfid, err := sfuuid.ToSFID(contactUID)
	if err != nil {
		sfid = contactUID
	}

	patchBody := buildKeyContactPatch(input)

	// If email is changing, resolve (or create) the new Contact and rewire Contact__c.
	if input.Email != nil {
		// Convert v2 B2BOrgUID back to SF Account ID for new-Contact creation; empty string
		// is fine for existing-contact resolution (accountSFID is only used on insert).
		accountSFID, _ := sfuuid.ToSFID(input.AccountSFID)
		contactSFID, created, resolveErr := w.contactResolver.ResolveOrCreateContact(
			ctx, *input.Email, input.FirstName, input.LastName, input.Title, accountSFID)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolving contact for email %q: %w", *input.Email, resolveErr)
		}
		if created {
			slog.InfoContext(ctx, "new Salesforce Contact created during key contact email update",
				"email", *input.Email, "contact_sfid", contactSFID)
		}
		patchBody["Contact__c"] = contactSFID
	}

	if len(patchBody) == 0 {
		// Nothing to update — return the current record unchanged.
		contact, err := w.contacts.FetchKeyContactBySFID(ctx, sfid)
		if err != nil {
			return nil, fmt.Errorf("fetching key contact %s: %w", contactUID, err)
		}
		if contact == nil {
			return nil, errs.NewNotFound("key contact not found", fmt.Errorf("uid: %s", contactUID))
		}
		// Stamp ProjectUID from the input so the caller receives a fully-populated
		// response without requiring an additional resolver round-trip.
		if input.ProjectUID != "" {
			contact.ProjectUID = input.ProjectUID
		}
		return contact, nil
	}

	patchJSON, err := json.Marshal(patchBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling key contact patch for %s: %w", contactUID, err)
	}

	uri := fmt.Sprintf("/services/data/%s/sobjects/Project_Role__c/%s", w.client.GetAPIVersion(), sfid)

	var resp *http.Response
	if err := retryOnLock(ctx, writerMaxRetries, writerRetryDelay, func() error {
		var writeErr error
		resp, writeErr = w.sobjectClient.DoConditionalWrite(ctx, http.MethodPatch, uri, patchJSON,
			"", input.IfUnmodifiedSince)
		return writeErr
	}); err != nil {
		return nil, fmt.Errorf("updating key contact %s in Salesforce: %w", contactUID, err)
	}

	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck
		switch resp.StatusCode {
		case http.StatusPreconditionFailed:
			return nil, errs.NewPreconditionFailed(
				fmt.Sprintf("key contact %s has been modified since last read (stale If-Match)", contactUID))
		case http.StatusOK, http.StatusNoContent:
			// Success — fall through to re-fetch.
		default:
			return nil, fmt.Errorf("updating key contact %s: unexpected Salesforce status %d", contactUID, resp.StatusCode)
		}
	}

	slog.DebugContext(ctx, "key contact updated in Salesforce", "sfid", sfid)

	w.invalidateKeyContactsCache(ctx, input.MembershipUID)

	contact, err := w.contacts.FetchKeyContactBySFID(ctx, sfid)
	if err != nil {
		return nil, fmt.Errorf("re-fetching updated key contact %s: %w", contactUID, err)
	}
	if contact == nil {
		return nil, errs.NewNotFound("key contact not found after update", fmt.Errorf("uid: %s", contactUID))
	}

	// Stamp ProjectUID from the input so the caller receives a fully-populated
	// response without requiring an additional resolver round-trip.
	if input.ProjectUID != "" {
		contact.ProjectUID = input.ProjectUID
	}

	return contact, nil
}

// DeleteKeyContact soft-deletes the Project_Role__c record identified by
// contactUID via the Salesforce REST delete endpoint. After a successful delete
// the key-contacts cache entry for the parent membership is deleted.
// membershipUID is used only for cache invalidation; pass an empty string to
// skip cache invalidation (the cache will expire naturally via bucket TTL).
//
// TODO: extend port signature to accept IfUnmodifiedSince for SF-side conditional
// delete (LFXV2-1362 follow-up). For now the DELETE is unconditional.
func (w *KeyContactWriter) DeleteKeyContact(ctx context.Context, contactUID string, membershipUID string) error {
	sfid, err := sfuuid.ToSFID(contactUID)
	if err != nil {
		sfid = contactUID
	}

	record := sfProjectRoleDelete{ID: sfid}

	if err := retryOnLock(ctx, writerMaxRetries, writerRetryDelay, func() error {
		return w.client.DeleteOne("Project_Role__c", record)
	}); err != nil {
		return fmt.Errorf("deleting key contact %s from Salesforce: %w", contactUID, err)
	}

	slog.DebugContext(ctx, "key contact deleted from Salesforce", "sfid", sfid)

	if membershipUID != "" {
		w.invalidateKeyContactsCache(ctx, membershipUID)
	}

	return nil
}

// buildKeyContactPatch constructs the JSON-serialisable PATCH body for a
// Salesforce Project_Role__c update. Only non-nil pointer fields from input are
// included; nil pointer fields are omitted so the existing values are preserved.
// Note: email updates are NOT supported via this patch function; email changes
// require Contact resolution and are handled separately in the service layer.
func buildKeyContactPatch(input model.KeyContactInput) map[string]any {
	patch := make(map[string]any)
	if input.Role != nil {
		patch["Role__c"] = *input.Role
	}
	if input.Status != nil {
		patch["Status__c"] = *input.Status
	}
	if input.BoardMember != nil {
		patch["BoardMember__c"] = *input.BoardMember
	}
	if input.PrimaryContact != nil {
		patch["PrimaryContact__c"] = *input.PrimaryContact
	}
	return patch
}

// invalidateKeyContactsCache deletes the key-contacts KV cache entry for the
// given membershipUID. Errors are logged at warn level and ignored — a stale
// cache entry will expire via the bucket TTL.
func (w *KeyContactWriter) invalidateKeyContactsCache(ctx context.Context, membershipUID string) {
	if membershipUID == "" {
		return
	}
	if err := w.cache.DeleteKeyContactsForMembership(ctx, membershipUID); err != nil {
		slog.WarnContext(ctx, "failed to invalidate key contacts cache after write",
			"membership_uid", membershipUID,
			"error", err,
		)
	}
}

// retryOnLock calls fn up to maxRetries times. If fn returns an error whose
// message contains entityIsLockedCode it waits baseDelay before retrying. Any
// other error is returned immediately without retrying. If all retries are
// exhausted the last error is returned.
func retryOnLock(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !strings.Contains(lastErr.Error(), entityIsLockedCode) {
			return lastErr
		}

		slog.WarnContext(ctx, "Salesforce record locked; retrying",
			"attempt", attempt+1,
			"max_retries", maxRetries,
			"error", lastErr,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(baseDelay):
		}
	}
	return lastErr
}
