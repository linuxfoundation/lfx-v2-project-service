// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
// The lfx-v2-project-service service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
)

const (
	// PS256 is the default for Heimdall's JWT finalizer.
	signatureAlgorithm = validator.PS256
	defaultIssuer      = "heimdall"
	defaultAudience    = "query-svc"
)

var (
	// Factory for custom JWT claims target.
	customClaims = func() validator.CustomClaims {
		return &HeimdallClaims{}
	}
)

// HeimdallClaims contains extra custom claims we want to parse from the JWT
// token.
type HeimdallClaims struct {
	Principal string `json:"principal"`
	Email     string `json:"email,omitempty"`
}

// Validate provides additional middleware validation of any claims defined in
// HeimdallClaims.
func (c *HeimdallClaims) Validate(ctx context.Context) error {
	if c.Principal == "" {
		return errors.New("principal must be provided")
	}
	return nil
}

type jwtAuth struct {
	validator *validator.Validator
}

func setupJWTAuth(logger *slog.Logger) *jwtAuth {
	// Set up Heimdall JWKS key provider.
	jwksEnv := os.Getenv("JWKS_URL")
	if jwksEnv == "" {
		jwksEnv = "http://heimdall:4457/.well-known/jwks"
	}
	jwksURL, err := url.Parse(jwksEnv)
	if err != nil {
		logger.With(errKey, err).Error("invalid JWKS_URL")
		os.Exit(1)
	}
	var issuer *url.URL
	issuer, err = url.Parse(defaultIssuer)
	if err != nil {
		// This shouldn't happen; a bare hostname is a valid URL.
		logger.Error("unexpected URL parsing of default issuer")
		os.Exit(1)
	}
	provider := jwks.NewCachingProvider(issuer, 5*time.Minute, jwks.WithCustomJWKSURI(jwksURL))

	// Set up the JWT validator.
	audience := os.Getenv("AUDIENCE")
	if audience == "" {
		audience = defaultAudience
	}
	jwtValidator, err := validator.New(
		provider.KeyFunc,
		signatureAlgorithm,
		issuer.String(),
		[]string{audience},
		validator.WithCustomClaims(customClaims),
		validator.WithAllowedClockSkew(5*time.Second),
	)
	if err != nil {
		logger.With(errKey, err).Error("failed to set up the Heimdall JWT validator")
		os.Exit(1)
	}

	return &jwtAuth{
		validator: jwtValidator,
	}
}

// parsePrincipal extracts the principal from the JWT claims.
func (j *jwtAuth) parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	if j.validator == nil {
		return "", errors.New("JWT validator is not set up")
	}

	parsedJWT, err := j.validator.ValidateToken(ctx, token)
	if err != nil {
		// Drop tertiary (and deeper) nested errors for security reasons. This is
		// using colons as an approximation for error nesting, which may not
		// exactly match to error boundaries. Unwrapping the error twice, then
		// dropping the suffix of the 3rd error's String() method could be more
		// accurate to error boundaries, but could also expose tertiary errors if
		// errors are not wrapped with Go 1.13 `%w` semantics.
		logger.With(errKey, err).WarnContext(ctx, "authorization failed")
		errString := err.Error()
		firstColon := strings.Index(errString, ":")
		if firstColon != -1 && firstColon+1 < len(errString) {
			errString = strings.Replace(errString, ": go-jose/go-jose/jwt", "", 1)
			secondColon := strings.Index(errString[firstColon+1:], ":")
			if secondColon != -1 {
				// Error has two colons (which may be 3 or more errors), so drop the
				// second colon and everything after it.
				errString = errString[:firstColon+secondColon+1]
			}
		}
		return "", errors.New(errString)
	}

	claims, ok := parsedJWT.(*validator.ValidatedClaims)
	if !ok {
		// This should never happen.
		return "", errors.New("failed to get validated authorization claims")
	}

	customClaims, ok := claims.CustomClaims.(*HeimdallClaims)
	if !ok {
		// This should never happen.
		return "", errors.New("failed to get custom authorization claims")
	}

	return customClaims.Principal, nil
}
