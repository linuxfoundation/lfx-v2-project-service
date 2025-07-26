// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

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
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	// PS256 is the default for Heimdall's JWT finalizer.
	signatureAlgorithm = validator.PS256
	defaultIssuer      = "heimdall"
	defaultAudience    = "lfx-v2-project-service"
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
func (c *HeimdallClaims) Validate(_ context.Context) error {
	if c.Principal == "" {
		return errors.New("principal must be provided")
	}
	return nil
}

type JWTAuth struct {
	validator *validator.Validator
}

// IJWTAuth is a JWT authentication interface needed for the [ProjectsService].
type IJWTAuth interface {
	ParsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error)
}

func NewJWTAuth() (*JWTAuth, error) {
	// Set up Heimdall JWKS key provider.
	jwksEnv := os.Getenv("JWKS_URL")
	if jwksEnv == "" {
		jwksEnv = "http://heimdall:4457/.well-known/jwks"
	}
	jwksURL, err := url.Parse(jwksEnv)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("invalid JWKS_URL")
		return nil, err
	}
	var issuer *url.URL
	issuer, err = url.Parse(defaultIssuer)
	if err != nil {
		// This shouldn't happen; a bare hostname is a valid URL.
		slog.Error("unexpected URL parsing of default issuer")
		return nil, err
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
		slog.With(constants.ErrKey, err).Error("failed to set up the Heimdall JWT validator")
		return nil, err
	}

	return &JWTAuth{
		validator: jwtValidator,
	}, nil
}

// ParsePrincipal extracts the principal from the JWT claims.
func (j *JWTAuth) ParsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	// To avoid having to use a valid JWT token for local development, we can set the
	// JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL environment variable to the principal
	// we want to use for local development.
	if mockLocalPrincipal := os.Getenv("JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL"); mockLocalPrincipal != "" {
		logger.InfoContext(ctx, "JWT authentication is disabled, returning mock principal",
			"principal", mockLocalPrincipal,
		)
		return mockLocalPrincipal, nil
	}

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
		logger.With("default_audience", defaultAudience).With("default_issuer", defaultIssuer).With(constants.ErrKey, err).WarnContext(ctx, "authorization failed")
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
