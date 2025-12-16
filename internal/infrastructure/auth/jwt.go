// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	// PS256 is the default signature algorithm used when JWT_SIGNATURE_ALGORITHM is not set.
	defaultSignatureAlgorithm = validator.PS256
	defaultIssuer             = "heimdall"
	defaultAudience           = "lfx-v2-project-service"
	defaultJWKSURL            = "http://heimdall:4457/.well-known/jwks"
)

// parseSignatureAlgorithm converts the algorithm string to a validator.SignatureAlgorithm.
// Returns PS256 as default if algoString is empty.
// Algorithm names are case-sensitive and must be uppercase (e.g., "PS256").
func parseSignatureAlgorithm(algoString string) (validator.SignatureAlgorithm, error) {
	if algoString == "" {
		return validator.PS256, nil
	}

	algorithms := map[string]validator.SignatureAlgorithm{
		"PS256": validator.PS256,
		"PS384": validator.PS384,
		"PS512": validator.PS512,
		"RS256": validator.RS256,
		"RS384": validator.RS384,
		"RS512": validator.RS512,
		"ES256": validator.ES256,
		"ES384": validator.ES384,
		"ES512": validator.ES512,
	}

	if algo, exists := algorithms[algoString]; exists {
		return algo, nil
	}

	return "", errors.New("unsupported JWT signature algorithm: " + algoString + " (supported: PS256, PS384, PS512, RS256, RS384, RS512, ES256, ES384, ES512)")
}

// JWTAuthConfig holds the configuration parameters for JWT authentication.
type JWTAuthConfig struct {
	// JWKSURL is the URL to the JSON Web Key Set endpoint
	JWKSURL string
	// Audience is the intended audience for the JWT token
	Audience string
	// MockLocalPrincipal is used for local development to bypass JWT validation
	MockLocalPrincipal string
	// SignatureAlgorithm is the JWT signature algorithm (e.g., PS256, RS256, ES256)
	SignatureAlgorithm string
}

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
	config    JWTAuthConfig
}

// Ensure JWTAuth implements domain.Authenticator interface
var _ domain.Authenticator = (*JWTAuth)(nil)

func NewJWTAuth(config JWTAuthConfig) (*JWTAuth, error) {
	// Parse signature algorithm
	algo, err := parseSignatureAlgorithm(config.SignatureAlgorithm)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("invalid JWT signature algorithm")
		return nil, err
	}

	// Log algorithm selection (especially if non-default)
	if config.SignatureAlgorithm != "" && config.SignatureAlgorithm != "PS256" {
		slog.Info("using non-default JWT signature algorithm",
			"algorithm", config.SignatureAlgorithm,
		)
	}

	// Set up defaults if not provided
	jwksURLStr := config.JWKSURL
	if jwksURLStr == "" {
		jwksURLStr = defaultJWKSURL
	}
	audience := config.Audience
	if audience == "" {
		audience = defaultAudience
	}

	// Set up Heimdall JWKS key provider.
	jwksURL, err := url.Parse(jwksURLStr)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("invalid JWKS_URL")
		return nil, err
	}
	var issuer *url.URL
	issuer, err = url.Parse(defaultIssuer)
	if err != nil {
		// This shouldn't happen; a bare hostname is a valid URL.
		slog.Error("unexpected URL parsing of default issuer", constants.ErrKey, err)
		return nil, err
	}
	provider := jwks.NewCachingProvider(issuer, 5*time.Minute, jwks.WithCustomJWKSURI(jwksURL))

	// Set up the JWT validator with selected algorithm.
	jwtValidator, err := validator.New(
		provider.KeyFunc,
		algo,
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
		config:    config,
	}, nil
}

// ParsePrincipal extracts the principal from the JWT claims.
func (j *JWTAuth) ParsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	// To avoid having to use a valid JWT token for local development, we can use the
	// MockLocalPrincipal configuration parameter.
	if j.config.MockLocalPrincipal != "" {
		logger.InfoContext(ctx, "JWT authentication is disabled, returning mock principal",
			"principal", j.config.MockLocalPrincipal,
		)
		return j.config.MockLocalPrincipal, nil
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
		logger.WarnContext(ctx, "authorization failed",
			"default_audience", defaultAudience,
			"default_issuer", defaultIssuer,
			constants.ErrKey, err,
		)
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

	logger.DebugContext(ctx, "JWT principal parsed",
		"principal", customClaims.Principal,
		"email", customClaims.Email,
	)

	return customClaims.Principal, nil
}
