// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package b2borg provides the B2BOrgResolver infrastructure implementation,
// which translates Salesforce Account SFIDs to v2 b2b_org UUIDs using a
// deterministic base-62 transform (no I/O required).
package b2borg

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// Resolver implements port.B2BOrgResolver via a deterministic base-62 transform.
// No I/O is required — the same SFID always maps to the same UID.
type Resolver struct{}

// Ensure Resolver satisfies the port at compile time.
var _ port.B2BOrgResolver = (*Resolver)(nil)

// NewResolver creates a new Resolver. No dependencies are required.
func NewResolver() *Resolver {
	return &Resolver{}
}

// UIDFromSFID resolves a Salesforce Account.Id to its v2 b2b_org UUID.
// The mapping is deterministic and lossless (see pkg/sfuuid). Returns a
// NotFound error when the input is empty or not a valid Salesforce SFID.
func (r *Resolver) UIDFromSFID(ctx context.Context, sfid string) (string, error) {
	sfid = strings.TrimSpace(sfid)

	if !sfuuid.IsSFID(sfid) {
		slog.DebugContext(ctx, "b2b-org resolver: invalid or unknown SFID",
			"sfid", sfid,
		)
		return "", errs.NewNotFound("b2b org not found", fmt.Errorf("sfid: %s", sfid))
	}

	uid, err := sfuuid.ToUUID(sfid)
	if err != nil {
		return "", errs.NewNotFound("b2b org not found", fmt.Errorf("sfid: %s: %w", sfid, err))
	}

	return uid, nil
}
