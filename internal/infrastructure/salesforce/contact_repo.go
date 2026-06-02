// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"
	"log/slog"

	sf "github.com/k-capehart/go-salesforce/v3"

	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// alternateEmailByAddressSOQL looks up a Contact SFID via Alternate_Email__c,
// matching any active email (not just primary). This mirrors the v1
// member-management lookup chain. The caller must substitute a quoteSOQL-escaped
// email address for the %s placeholder.
const alternateEmailByAddressSOQL = `
SELECT Contact_Name__c
FROM Alternate_Email__c
WHERE Alternate_Email_Address__c = %s
  AND Active__c = true
  AND IsDeleted = false
LIMIT 1
`

// contactByEmailSOQL falls back to Contact.Email when no Alternate_Email__c
// record exists yet (handles contacts so new that the sync trigger hasn't fired).
// The caller must substitute a quoteSOQL-escaped email address for %s.
const contactByEmailSOQL = `
SELECT Id
FROM Contact
WHERE Email = %s
  AND IsDeleted = false
LIMIT 1
`

// sfContact is the Salesforce DML struct used when creating a new Contact in
// the B2B org. Fields use salesforce tags matching the Salesforce API field names.
type sfContact struct {
	FirstName string `salesforce:"FirstName"`
	LastName  string `salesforce:"LastName"`
	Email     string `salesforce:"Email"`
	Title     string `salesforce:"Title,omitempty"`
	AccountID string `salesforce:"AccountId,omitempty"`
}

// ContactRepo handles Salesforce lookups and create-on-miss for Contact records
// in the B2B org.
type ContactRepo struct {
	client *sf.Salesforce
}

// NewContactRepo creates a new ContactRepo backed by the given Salesforce client.
func NewContactRepo(client *sf.Salesforce) *ContactRepo {
	return &ContactRepo{client: client}
}

// ResolveOrCreateContact resolves an email address to a Salesforce Contact.Id,
// creating a new Contact if none is found. Returns the Contact SFID and a
// boolean indicating whether a new Contact was inserted (created == true).
//
// accountSFID is optional: when non-empty it is set as the new Contact's
// AccountId so the record is associated with the correct company. It has no
// effect when an existing Contact is found (steps 1 or 2).
//
// Resolution chain:
//  1. SOQL lookup against Alternate_Email__c (any active email).
//  2. Fallback to Contact.Email (race-condition safety for very new contacts).
//  3. Insert a new Contact when neither lookup yields a result.
func (r *ContactRepo) ResolveOrCreateContact(
	ctx context.Context,
	email, firstName, lastName, title, accountSFID string,
) (contactSFID string, created bool, err error) {
	quotedEmail := quoteSOQL(email)

	// ── Step 1: Alternate_Email__c lookup ────────────────────────────────────
	var altEmails []soqlAlternateEmailContact
	if queryErr := r.client.Query(fmt.Sprintf(alternateEmailByAddressSOQL, quotedEmail), &altEmails); queryErr != nil {
		return "", false, fmt.Errorf("looking up contact via Alternate_Email__c for %q: %w", email, queryErr)
	}

	if len(altEmails) > 0 && altEmails[0].ContactNameID != "" {
		normalized, normErr := normalizeUID("Contact", altEmails[0].ContactNameID)
		if normErr != nil {
			return "", false, normErr
		}
		slog.DebugContext(ctx, "contact resolved via Alternate_Email__c",
			"email", email,
			"contact_sfid", normalized,
		)
		return normalized, false, nil
	}

	// ── Step 2: Contact.Email fallback ───────────────────────────────────────
	var contacts []soqlContactByEmail
	if queryErr := r.client.Query(fmt.Sprintf(contactByEmailSOQL, quotedEmail), &contacts); queryErr != nil {
		return "", false, fmt.Errorf("looking up contact via Contact.Email for %q: %w", email, queryErr)
	}

	if len(contacts) > 0 && contacts[0].ID != "" {
		normalized, normErr := normalizeUID("Contact", contacts[0].ID)
		if normErr != nil {
			return "", false, normErr
		}
		slog.DebugContext(ctx, "contact resolved via Contact.Email fallback",
			"email", email,
			"contact_sfid", normalized,
		)
		return normalized, false, nil
	}

	// ── Step 3: Create a new Contact ─────────────────────────────────────────
	slog.InfoContext(ctx, "no existing contact found; creating new Contact in Salesforce",
		"email", email,
		"first_name", firstName,
		"last_name", lastName,
	)

	newContact := sfContact{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Title:     title,
		AccountID: accountSFID,
	}

	var result sf.SalesforceResult
	if insertErr := retryOnLock(ctx, writerMaxRetries, writerRetryDelay, func() error {
		var e error
		result, e = r.client.InsertOne("Contact", newContact)
		return e
	}); insertErr != nil {
		return "", false, fmt.Errorf("creating new Contact for %q in Salesforce: %w", email, insertErr)
	}

	if !result.Success || result.Id == "" {
		return "", false, errs.NewUnexpected(
			fmt.Sprintf("Salesforce insert for Contact %q reported failure without an error", email),
		)
	}

	normalized, normErr := normalizeUID("Contact", result.Id)
	if normErr != nil {
		return "", false, normErr
	}

	slog.InfoContext(ctx, "new Contact created in Salesforce",
		"email", email,
		"contact_sfid", normalized,
	)

	return normalized, true, nil
}
