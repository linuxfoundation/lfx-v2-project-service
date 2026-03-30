// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package port defines the domain interfaces (ports) that infrastructure
// implementations must satisfy.
package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// KeyContactWriter provides write access to Project_Role__c key contact records
// in Salesforce. Implementations are responsible for cache invalidation after
// successful mutations.
type KeyContactWriter interface {
	// CreateKeyContact creates a new Project_Role__c record in Salesforce and
	// returns the resulting domain object. The input must include a valid
	// Email, FirstName, LastName, and MembershipUID; all pointer fields default
	// to zero values if nil. The writer resolves (or creates) the B2B Salesforce
	// Contact.Id internally from the email address.
	CreateKeyContact(ctx context.Context, input model.KeyContactInput) (*model.ProjectKeyContact, error)

	// UpdateKeyContact updates the mutable fields of an existing
	// Project_Role__c record identified by contactUID. Only non-nil pointer
	// fields in input are applied; nil fields are left unchanged.
	UpdateKeyContact(ctx context.Context, contactUID string, input model.KeyContactInput) (*model.ProjectKeyContact, error)

	// DeleteKeyContact soft-deletes the Project_Role__c record identified by
	// contactUID. membershipUID is used to invalidate the key-contacts KV cache
	// entry for the parent membership.
	DeleteKeyContact(ctx context.Context, contactUID string, membershipUID string) error
}
