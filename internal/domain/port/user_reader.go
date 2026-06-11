// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

import "context"

// UserReader provides access to user identity information from auth-service.
type UserReader interface {
	// UsernameByEmail resolves the registered LFID username for the given primary email address.
	// Returns NotFound if the email does not exist in auth-service.
	UsernameByEmail(ctx context.Context, email string) (string, error)
}
