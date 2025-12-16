// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeimdallClaims_Validate(t *testing.T) {
	tests := []struct {
		name    string
		claims  HeimdallClaims
		wantErr bool
	}{
		{
			name: "valid claims with principal",
			claims: HeimdallClaims{
				Principal: "user123",
				Email:     "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid claims with principal only",
			claims: HeimdallClaims{
				Principal: "user456",
			},
			wantErr: false,
		},
		{
			name: "invalid claims without principal",
			claims: HeimdallClaims{
				Email: "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid claims with empty principal",
			claims: HeimdallClaims{
				Principal: "",
				Email:     "test@example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.claims.Validate(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "principal must be provided")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewJWTAuth(t *testing.T) {
	tests := []struct {
		name      string
		config    JWTAuthConfig
		wantErr   bool
		expectNil bool
	}{
		{
			name:   "successful initialization with defaults",
			config: JWTAuthConfig{
				// Use default values (empty strings)
			},
			wantErr:   false,
			expectNil: false,
		},
		{
			name: "successful initialization with custom values",
			config: JWTAuthConfig{
				JWKSURL:  "http://custom-jwks:4457/.well-known/jwks",
				Audience: "custom-audience",
			},
			wantErr:   false,
			expectNil: false,
		},
		{
			name: "invalid JWKS URL",
			config: JWTAuthConfig{
				JWKSURL: "://invalid-url",
			},
			wantErr:   true,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewJWTAuth(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, auth)
			} else {
				assert.NotNil(t, auth)
				if auth != nil {
					assert.NotNil(t, auth.validator)
					assert.Equal(t, tt.config, auth.config)
				}
			}
		})
	}
}

func TestJWTAuth_ParsePrincipal_MockMode(t *testing.T) {
	tests := []struct {
		name               string
		mockLocalPrincipal string
		token              string
		expected           string
		wantErr            bool
	}{
		{
			name:               "mock mode with valid principal",
			mockLocalPrincipal: "test-user-123",
			token:              "any-token",
			expected:           "test-user-123",
			wantErr:            false,
		},
		{
			name:               "mock mode with empty principal",
			mockLocalPrincipal: "",
			token:              "any-token",
			expected:           "",
			wantErr:            true,
		},
		{
			name:               "production mode without mock",
			mockLocalPrincipal: "",
			token:              "invalid-token",
			expected:           "",
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &JWTAuth{
				validator: nil, // Mock mode doesn't use validator
				config: JWTAuthConfig{
					MockLocalPrincipal: tt.mockLocalPrincipal,
				},
			}

			logger := slog.Default()
			principal, err := auth.ParsePrincipal(context.Background(), tt.token, logger)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, principal)
			}
		})
	}
}

func TestJWTAuth_ParsePrincipal_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		auth      *JWTAuth
		token     string
		wantErr   bool
		errString string
	}{
		{
			name: "nil validator",
			auth: &JWTAuth{
				validator: nil,
				config:    JWTAuthConfig{}, // No mock principal
			},
			token:     "some-token",
			wantErr:   true,
			errString: "JWT validator is not set up",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No mock principal set in config

			logger := slog.Default()
			principal, err := tt.auth.ParsePrincipal(context.Background(), tt.token, logger)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				assert.Empty(t, principal)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, principal)
			}
		})
	}
}

func TestJWTAuth_Constants(t *testing.T) {
	t.Run("verify constants", func(t *testing.T) {
		assert.Equal(t, "heimdall", defaultIssuer)
		assert.Equal(t, "lfx-v2-project-service", defaultAudience)
		assert.Equal(t, "http://heimdall:4457/.well-known/jwks", defaultJWKSURL)
		assert.NotNil(t, defaultSignatureAlgorithm)
	})
}

func TestJWTAuth_CustomClaimsFactory(t *testing.T) {
	t.Run("custom claims factory creates HeimdallClaims", func(t *testing.T) {
		claims := customClaims()
		assert.NotNil(t, claims)

		// Verify it's the correct type
		heimdallClaims, ok := claims.(*HeimdallClaims)
		assert.True(t, ok)
		assert.NotNil(t, heimdallClaims)

		// Test the Validate method
		err := heimdallClaims.Validate(context.Background())
		assert.Error(t, err) // Should error because Principal is empty

		// Set principal and test again
		heimdallClaims.Principal = "test-principal"
		err = heimdallClaims.Validate(context.Background())
		assert.NoError(t, err)
	})
}

func TestJWTAuth_Integration(t *testing.T) {
	t.Run("end to end mock authentication", func(t *testing.T) {
		// Create auth instance with mock config
		auth := &JWTAuth{
			validator: nil,
			config: JWTAuthConfig{
				MockLocalPrincipal: "integration-test-user",
			},
		}

		// Test parsing
		ctx := context.Background()
		logger := slog.Default()
		principal, err := auth.ParsePrincipal(ctx, "fake-token", logger)

		assert.NoError(t, err)
		assert.Equal(t, "integration-test-user", principal)
	})
}

func TestJWTAuth_ConfigurationHandling(t *testing.T) {
	tests := []struct {
		name        string
		config      JWTAuthConfig
		shouldError bool
		description string
	}{
		{
			name:        "empty config uses defaults",
			config:      JWTAuthConfig{},
			shouldError: false,
			description: "should use defaults",
		},
		{
			name: "custom JWKS URL set",
			config: JWTAuthConfig{
				JWKSURL: "http://localhost:4457/.well-known/jwks",
			},
			shouldError: false,
			description: "should accept custom JWKS URL",
		},
		{
			name: "custom audience set",
			config: JWTAuthConfig{
				Audience: "custom-service",
			},
			shouldError: false,
			description: "should accept custom audience",
		},
		{
			name: "both custom values set",
			config: JWTAuthConfig{
				JWKSURL:  "http://localhost:4457/.well-known/jwks",
				Audience: "custom-service",
			},
			shouldError: false,
			description: "should accept both custom values",
		},
		{
			name: "mock principal configured",
			config: JWTAuthConfig{
				MockLocalPrincipal: "test-user",
			},
			shouldError: false,
			description: "should accept mock principal",
		},
		{
			name: "custom signature algorithm ES256",
			config: JWTAuthConfig{
				SignatureAlgorithm: "ES256",
			},
			shouldError: false,
			description: "should accept valid signature algorithm",
		},
		{
			name: "custom signature algorithm RS256",
			config: JWTAuthConfig{
				SignatureAlgorithm: "RS256",
			},
			shouldError: false,
			description: "should accept RS256 signature algorithm",
		},
		{
			name: "invalid signature algorithm",
			config: JWTAuthConfig{
				SignatureAlgorithm: "INVALID",
			},
			shouldError: true,
			description: "should reject invalid signature algorithm",
		},
		{
			name: "lowercase signature algorithm rejected",
			config: JWTAuthConfig{
				SignatureAlgorithm: "ps256",
			},
			shouldError: true,
			description: "should reject lowercase signature algorithm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewJWTAuth(tt.config)

			if tt.shouldError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, auth, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, auth, tt.description)
				if auth != nil {
					assert.Equal(t, tt.config, auth.config)
				}
			}
		})
	}
}

func TestParseSignatureAlgorithm(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		wantErr   bool
	}{
		// Valid algorithms - PS family
		{name: "PS256 valid", algorithm: "PS256", wantErr: false},
		{name: "PS384 valid", algorithm: "PS384", wantErr: false},
		{name: "PS512 valid", algorithm: "PS512", wantErr: false},

		// Valid algorithms - RS family
		{name: "RS256 valid", algorithm: "RS256", wantErr: false},
		{name: "RS384 valid", algorithm: "RS384", wantErr: false},
		{name: "RS512 valid", algorithm: "RS512", wantErr: false},

		// Valid algorithms - ES family
		{name: "ES256 valid", algorithm: "ES256", wantErr: false},
		{name: "ES384 valid", algorithm: "ES384", wantErr: false},
		{name: "ES512 valid", algorithm: "ES512", wantErr: false},

		// Empty string uses default
		{name: "empty defaults to PS256", algorithm: "", wantErr: false},

		// Invalid - case sensitivity
		{name: "lowercase rejected", algorithm: "ps256", wantErr: true},
		{name: "mixed case rejected", algorithm: "Ps256", wantErr: true},

		// Invalid - HMAC algorithms not supported
		{name: "HS256 unsupported", algorithm: "HS256", wantErr: true},
		{name: "HS384 unsupported", algorithm: "HS384", wantErr: true},
		{name: "HS512 unsupported", algorithm: "HS512", wantErr: true},

		// Invalid - unknown algorithms
		{name: "unknown algorithm", algorithm: "UNKNOWN", wantErr: true},
		{name: "typo", algorithm: "PS265", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algo, err := parseSignatureAlgorithm(tt.algorithm)
			if tt.wantErr {
				assert.Error(t, err, "expected error for algorithm %q", tt.algorithm)
				assert.Empty(t, algo, "expected empty algorithm for %q", tt.algorithm)
			} else {
				assert.NoError(t, err, "unexpected error for algorithm %q", tt.algorithm)
				assert.NotEmpty(t, algo, "expected valid algorithm for %q", tt.algorithm)
			}
		})
	}
}
