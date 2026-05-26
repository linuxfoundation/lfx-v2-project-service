// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// B2BOrgResolver translates a Salesforce Account SFID into its v2 b2b_org UID.
// Implementations may use a deterministic transform, a KV cache lookup, or a
// Salesforce round-trip; callers should treat all errors as "not found".
type B2BOrgResolver interface {
	// UIDFromSFID resolves a Salesforce Account.Id to a v2 b2b_org UID.
	// Returns a NotFound error if the SFID is malformed or unknown.
	UIDFromSFID(ctx context.Context, sfid string) (uid string, err error)
}
