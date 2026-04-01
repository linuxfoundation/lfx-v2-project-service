// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import (
	"context"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

// B2BOrgWriter provides write access to B2BOrg (Salesforce Account) records.
// Implementations are responsible for cache invalidation after successful
// mutations.
type B2BOrgWriter interface {
	// CreateB2BOrg creates a new B2BOrg record derived from the given
	// Salesforce Account.Id. The SFID is used to fetch the Account fields and
	// derive the v2 UUID. Returns the resulting domain object.
	CreateB2BOrg(ctx context.Context, sfid string) (*model.B2BOrg, error)

	// UpdateB2BOrg updates the mutable fields of an existing B2BOrg record
	// identified by its v2 UUID. Only non-zero fields in input are applied;
	// zero-value fields are left unchanged.
	UpdateB2BOrg(ctx context.Context, uid string, input model.B2BOrgInput) (*model.B2BOrg, error)
}
