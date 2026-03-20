// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package salesforce provides a Salesforce SOQL-backed implementation of
// port.MemberReader. Salesforce is the source of truth; NATS KV is used as a
// per-record TTL cache in front of it. Cache misses are transparent to callers.
package salesforce

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sf "github.com/k-capehart/go-salesforce/v3"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
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
	contacts        *KeyContactRepo
	contactResolver *ContactRepo
	cache           *nats.Storage
}

// NewKeyContactWriter creates a new KeyContactWriter backed by the given
// Salesforce client, key contact read repo (for post-write re-fetch), contact
// resolver (for email → Contact SFID resolution on create), and NATS KV cache
// (for invalidation).
func NewKeyContactWriter(
	client *sf.Salesforce,
	contacts *KeyContactRepo,
	contactResolver *ContactRepo,
	cache *nats.Storage,
) *KeyContactWriter {
	return &KeyContactWriter{
		client:          client,
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
func (w *KeyContactWriter) CreateKeyContact(ctx context.Context, input model.KeyContactInput) (*model.ProjectKeyContact, error) {
	assetSFID, err := sfuuid.ToSFID(input.MembershipUID)
	if err != nil {
		// Treat a value that does not decode as an LFX_ UUID as a raw SFID.
		assetSFID = input.MembershipUID
	}

	contactSFID, created, err := w.contactResolver.ResolveOrCreateContact(
		ctx,
		input.Email,
		input.FirstName,
		input.LastName,
		input.Title,
		input.AccountSFID,
	)
	if err != nil {
		return nil, fmt.Errorf("resolving contact for email %q: %w", input.Email, err)
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
// values are preserved. After a successful update the key-contacts cache entry
// for the parent membership is deleted and the updated record is re-fetched.
func (w *KeyContactWriter) UpdateKeyContact(ctx context.Context, contactUID string, input model.KeyContactInput) (*model.ProjectKeyContact, error) {
	sfid, err := sfuuid.ToSFID(contactUID)
	if err != nil {
		sfid = contactUID
	}

	record := sfProjectRole{
		ID:             sfid,
		Role:           input.Role,
		Status:         input.Status,
		BoardMember:    input.BoardMember,
		PrimaryContact: input.PrimaryContact,
	}

	if err := retryOnLock(ctx, writerMaxRetries, writerRetryDelay, func() error {
		return w.client.UpdateOne("Project_Role__c", record)
	}); err != nil {
		return nil, fmt.Errorf("updating key contact %s in Salesforce: %w", contactUID, err)
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
