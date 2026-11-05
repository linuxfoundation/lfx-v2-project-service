// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

// AuthIdentity is the identity and optional source extracted from a validated token.
// Origin identifies the calling integration, not the impersonated human.
type AuthIdentity struct {
	Principal string
	Email     string
	Origin    string
}

// Authenticator defines the authentication interface for the domain layer.
// This interface allows the service layer to authenticate users without depending
// on specific authentication implementations (JWT, OAuth, etc.).
// Implementations that need to log capture a logger at construction (e.g. slog.Default()).
type Authenticator interface {
	// ParseIdentity validates a token and extracts its principal, email, and optional origin.
	ParseIdentity(ctx context.Context, token string) (AuthIdentity, error)
	// ParsePrincipal extracts the principal (user identifier) from an authentication token.
	// Returns the principal string and any error that occurred during parsing.
	ParsePrincipal(ctx context.Context, token string) (string, error)
	// ParsePrincipalAndEmail extracts the principal and, when present, the email claim.
	// Email may be empty even on success.
	ParsePrincipalAndEmail(ctx context.Context, token string) (principal, email string, err error)
}
