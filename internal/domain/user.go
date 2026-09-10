// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// UserMetadata holds the profile fields the project service actually reads.
// Only Name, GivenName, FamilyName, and Picture are used for display-name
// resolution and audit stamping. The auth service returns many more fields
// that this service has no need for; they are intentionally omitted here.
type UserMetadata struct {
	Name       string
	GivenName  string
	FamilyName string
	Picture    string
}

// UserReader retrieves user profile information from the auth service.
type UserReader interface {
	// UserMetadataByPrincipal retrieves profile metadata for a user by their principal.
	UserMetadataByPrincipal(ctx context.Context, principal string) (*UserMetadata, error)
	// UsernameByEmail resolves the registered LFID username for the given primary email address.
	// Returns ErrUserNotFound when no user is registered with that email.
	UsernameByEmail(ctx context.Context, email string) (string, error)
	// PrimaryEmailByUsername resolves the user's primary email via auth-service user_emails.read.
	// Returns empty string when the user has no primary email; errors indicate transport or parsing failures.
	PrimaryEmailByUsername(ctx context.Context, username string) (string, error)
}
