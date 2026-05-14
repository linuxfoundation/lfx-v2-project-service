// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// UserMetadata holds profile information for a user returned by the auth service.
type UserMetadata struct {
	Name       string
	GivenName  string
	FamilyName string
}

// UserReader retrieves user profile information from the auth service.
type UserReader interface {
	// UserMetadataByPrincipal retrieves profile metadata for a user by their principal.
	UserMetadataByPrincipal(ctx context.Context, principal string) (*UserMetadata, error)
}
