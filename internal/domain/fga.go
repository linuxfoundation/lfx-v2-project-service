// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// AccessChecker verifies whether a user holds a given relation on an FGA object.
// The object must be in the form "type:id" (e.g. "project:abc-123").
// The user must be in the form "user:username".
// Implementations must be safe for concurrent use.
type AccessChecker interface {
	Check(ctx context.Context, user, relation, object string) (bool, error)
}
