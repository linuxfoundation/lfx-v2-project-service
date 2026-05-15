// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// B2BOrgReader implements port.B2BOrgReader using the Salesforce sObject REST
// API as the source of truth and NATS KV as a per-record conditional-GET cache.
type B2BOrgReader struct {
	client *SObjectClient
}

// NewB2BOrgReader creates a B2BOrgReader backed by the given SObjectClient.
func NewB2BOrgReader(client *SObjectClient) *B2BOrgReader {
	return &B2BOrgReader{client: client}
}

// Ensure B2BOrgReader satisfies the port at compile time.
var _ port.B2BOrgReader = (*B2BOrgReader)(nil)

// GetB2BOrg returns the B2BOrg identified by its v2 UUID. Returns an error
// wrapping ErrNotFound if no record exists.
func (r *B2BOrgReader) GetB2BOrg(ctx context.Context, uid string) (*model.B2BOrg, error) {
	org, _, err := r.client.FetchB2BOrg(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("fetching b2b org %s from Salesforce: %w", uid, err)
	}
	if org == nil {
		return nil, errs.NewNotFound("b2b org not found", fmt.Errorf("uid: %s", uid))
	}
	return org, nil
}
