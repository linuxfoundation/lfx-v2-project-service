// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// B2BOrgReader implements port.B2BOrgReader using Salesforce SOQL as the
// source of truth and NATS KV as a per-record TTL cache.
type B2BOrgReader struct {
	accounts *AccountRepo
	cache    *nats.Storage
}

// NewB2BOrgReader creates a B2BOrgReader backed by the given AccountRepo and
// NATS KV cache.
func NewB2BOrgReader(accounts *AccountRepo, cache *nats.Storage) *B2BOrgReader {
	return &B2BOrgReader{accounts: accounts, cache: cache}
}

// Ensure B2BOrgReader satisfies the port at compile time.
var _ port.B2BOrgReader = (*B2BOrgReader)(nil)

// ─── GetB2BOrg ───────────────────────────────────────────────────────────────

// GetB2BOrg returns the B2BOrg identified by its v2 UUID. Returns an error
// wrapping ErrNotFound if no record exists.
func (r *B2BOrgReader) GetB2BOrg(ctx context.Context, uid string) (*model.B2BOrg, error) {
	sfid, err := sfuuid.ToSFID(uid)
	if err != nil {
		return nil, fmt.Errorf("decoding b2b org UID %s: %w", uid, err)
	}

	org, err := r.accounts.FetchAccountBySFID(ctx, sfid)
	if err != nil {
		return nil, fmt.Errorf("fetching b2b org %s from Salesforce: %w", uid, err)
	}
	if org == nil {
		return nil, errs.NewNotFound("b2b org not found", fmt.Errorf("uid: %s", uid))
	}

	return org, nil
}
