// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package b2borg provides the B2BOrgResolver infrastructure implementation,
// which validates and normalises Salesforce Account SFIDs for use as b2b_org
// uid values. The uid for a b2b_org is its 18-char Salesforce Account.Id.
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

// Resolver implements port.B2BOrgResolver. No I/O is required — the uid for a
// b2b_org is its canonical 18-char Salesforce Account.Id.
type Resolver struct{}

// Ensure Resolver satisfies the port at compile time.
var _ port.B2BOrgResolver = (*Resolver)(nil)

// NewResolver creates a new Resolver. No dependencies are required.
func NewResolver() *Resolver {
	return &Resolver{}
}

// UIDFromSFID resolves a Salesforce Account.Id to its b2b_org uid, which is
// the canonical 18-char SFID itself. Returns a NotFound error when the input is
// empty or not a valid Salesforce SFID.
func (r *Resolver) UIDFromSFID(ctx context.Context, sfid string) (string, error) {
	sfid = strings.TrimSpace(sfid)

	uid, err := sfuuid.Normalize18(sfid)
	if err != nil {
		slog.DebugContext(ctx, "b2b-org resolver: invalid or unknown SFID", "sfid", sfid)
		return "", errs.NewNotFound("b2b org not found", fmt.Errorf("sfid: %s: %w", sfid, err))
	}

	return uid, nil
}
