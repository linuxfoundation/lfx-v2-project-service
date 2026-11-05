// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
					assert.NotNil(t, auth.logger,
						"NewJWTAuth must initialize logger; nil *slog.Logger panics on first log call")
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
				logger: slog.Default(),
			}

			principal, err := auth.ParsePrincipal(context.Background(), tt.token)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, principal)
			}
		})
	}
}

func TestJWTAuth_ParsePrincipalAndEmail_MockMode(t *testing.T) {
	auth := &JWTAuth{
		validator: nil,
		config: JWTAuthConfig{
			MockLocalPrincipal: "test-user-123",
		},
		logger: slog.Default(),
	}

	principal, email, err := auth.ParsePrincipalAndEmail(context.Background(), "any-token")

	require.NoError(t, err)
	assert.Equal(t, "test-user-123", principal)
	assert.Empty(t, email)
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
			tt.auth.logger = slog.Default()
			principal, err := tt.auth.ParsePrincipal(context.Background(), tt.token)

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
		assert.NotNil(t, signatureAlgorithm)
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
			logger: slog.Default(),
		}

		// Test parsing
		ctx := context.Background()
		principal, err := auth.ParsePrincipal(ctx, "fake-token")

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

func TestJWTAuth_ParseIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwtValidator, err := validator.New(
		func(context.Context) (interface{}, error) { return &key.PublicKey, nil },
		signatureAlgorithm, defaultIssuer, []string{defaultAudience},
		validator.WithCustomClaims(customClaims),
	)
	require.NoError(t, err)
	auth := &JWTAuth{validator: jwtValidator, logger: slog.Default()}

	tests := []struct {
		name     string
		origin   any
		omit     bool
		mutate   func(map[string]any)
		wrongKey bool
		mock     bool
		wantErr  bool
	}{
		{name: "signed origin with human attribution", origin: events.OriginV1SyncHelper},
		{name: "legacy absent claim", omit: true},
		{name: "empty claim", origin: ""},
		{name: "null is unspecified", origin: nil},
		{name: "unknown origin", origin: "other_source", wantErr: true},
		{name: "non-string origin", origin: true, wantErr: true},
		{name: "array origin", origin: []string{events.OriginV1SyncHelper}, wantErr: true},
		{name: "invalid signature", origin: events.OriginV1SyncHelper, wrongKey: true, wantErr: true},
		{name: "wrong issuer", origin: events.OriginV1SyncHelper, mutate: func(c map[string]any) { c["iss"] = "untrusted" }, wantErr: true},
		{name: "wrong audience", origin: events.OriginV1SyncHelper, mutate: func(c map[string]any) { c["aud"] = "other-service" }, wantErr: true},
		{name: "expired token", origin: events.OriginV1SyncHelper, mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, wantErr: true},
		{name: "missing principal", origin: events.OriginV1SyncHelper, mutate: func(c map[string]any) { delete(c, "principal") }, wantErr: true},
		{name: "mock bypass never trusts token origin", origin: events.OriginV1SyncHelper, mock: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := map[string]any{
				"iss": defaultIssuer, "aud": defaultAudience,
				"exp":       time.Now().Add(time.Minute).Unix(),
				"principal": "test-user", "email": "test@example.com",
			}
			if !tt.omit {
				claims[events.OriginClaim] = tt.origin
			}
			if tt.mutate != nil {
				tt.mutate(claims)
			}
			header, err := json.Marshal(map[string]string{"alg": "PS256", "typ": "JWT"})
			require.NoError(t, err)
			payload, err := json.Marshal(claims)
			require.NoError(t, err)
			input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
			digest := sha256.Sum256([]byte(input))
			signingKey := key
			if tt.wrongKey {
				signingKey = wrongKey
			}
			signature, err := rsa.SignPSS(rand.Reader, signingKey, crypto.SHA256, digest[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
			require.NoError(t, err)
			token := input + "." + base64.RawURLEncoding.EncodeToString(signature)
			auth.config.MockLocalPrincipal = ""
			if tt.mock {
				auth.config.MockLocalPrincipal = "test-user"
			}
			identity, err := auth.ParseIdentity(t.Context(), token)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, domain.AuthIdentity{}, identity)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "test-user", identity.Principal)
			if tt.mock {
				assert.Empty(t, identity.Email)
				assert.Empty(t, identity.Origin)
			} else {
				assert.Equal(t, "test@example.com", identity.Email)
				wantOrigin, _ := tt.origin.(string)
				assert.Equal(t, wantOrigin, identity.Origin)
			}
		})
	}
}
