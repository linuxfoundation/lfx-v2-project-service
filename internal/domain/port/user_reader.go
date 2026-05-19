// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// UserReader provides access to user identity information from auth-service.
type UserReader interface {
	// SubByEmail resolves an email address to its OIDC subject identifier.
	// Returns NotFound if the email does not exist in auth-service.
	SubByEmail(ctx context.Context, email string) (string, error)
}
