// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainErrors(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedMsg   string
		shouldBeError bool
	}{
		{
			name:          "project not found error",
			err:           ErrProjectNotFound,
			expectedMsg:   "project not found",
			shouldBeError: true,
		},
		{
			name:          "project slug exists error",
			err:           ErrProjectSlugExists,
			expectedMsg:   "project slug already exists",
			shouldBeError: true,
		},
		{
			name:          "internal error",
			err:           ErrInternal,
			expectedMsg:   "internal error",
			shouldBeError: true,
		},
		{
			name:          "revision mismatch error",
			err:           ErrRevisionMismatch,
			expectedMsg:   "revision mismatch",
			shouldBeError: true,
		},
		{
			name:          "unmarshal error",
			err:           ErrUnmarshal,
			expectedMsg:   "unmarshal error",
			shouldBeError: true,
		},
		{
			name:          "service unavailable error",
			err:           ErrServiceUnavailable,
			expectedMsg:   "service unavailable",
			shouldBeError: true,
		},
		{
			name:          "validation failed error",
			err:           ErrValidationFailed,
			expectedMsg:   "validation failed",
			shouldBeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedMsg, tt.err.Error())
			assert.Equal(t, tt.shouldBeError, tt.err != nil)
		})
	}
}

func TestErrorComparison(t *testing.T) {
	tests := []struct {
		name     string
		err1     error
		err2     error
		areEqual bool
	}{
		{
			name:     "same error instances",
			err1:     ErrProjectNotFound,
			err2:     ErrProjectNotFound,
			areEqual: true,
		},
		{
			name:     "different error instances",
			err1:     ErrProjectNotFound,
			err2:     ErrProjectSlugExists,
			areEqual: false,
		},
		{
			name:     "error vs nil",
			err1:     ErrInternal,
			err2:     nil,
			areEqual: false,
		},
		{
			name:     "both nil",
			err1:     nil,
			err2:     nil,
			areEqual: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			areEqual := errors.Is(tt.err1, tt.err2)
			assert.Equal(t, tt.areEqual, areEqual)
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	tests := []struct {
		name        string
		baseErr     error
		wrappedErr  error
		shouldMatch bool
	}{
		{
			name:        "wrapped project not found",
			baseErr:     ErrProjectNotFound,
			wrappedErr:  errors.New("repository error: project not found"),
			shouldMatch: false, // Not using fmt.Errorf with %w
		},
		{
			name:        "unwrapped error comparison",
			baseErr:     ErrInternal,
			wrappedErr:  ErrInternal,
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := errors.Is(tt.wrappedErr, tt.baseErr)
			assert.Equal(t, tt.shouldMatch, matches)
		})
	}
}

func TestErrorMessages(t *testing.T) {
	// Verify error messages are user-friendly and consistent
	tests := []struct {
		name        string
		err         error
		shouldMatch bool
		pattern     string
	}{
		{
			name:        "project not found message",
			err:         ErrProjectNotFound,
			shouldMatch: true,
			pattern:     "project not found",
		},
		{
			name:        "slug exists message",
			err:         ErrProjectSlugExists,
			shouldMatch: true,
			pattern:     "project slug already exists",
		},
		{
			name:        "validation failed message",
			err:         ErrValidationFailed,
			shouldMatch: true,
			pattern:     "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := tt.err.Error()
			contains := message == tt.pattern
			assert.Equal(t, tt.shouldMatch, contains)
		})
	}
}

func TestErrorTypes(t *testing.T) {
	// Verify all domain errors implement the error interface
	domainErrors := []error{
		ErrProjectNotFound,
		ErrProjectSlugExists,
		ErrInternal,
		ErrRevisionMismatch,
		ErrUnmarshal,
		ErrServiceUnavailable,
		ErrValidationFailed,
	}

	for i, err := range domainErrors {
		t.Run("error_implements_interface_"+string(rune(i+'0')), func(t *testing.T) {
			// Verify it implements error interface
			assert.Implements(t, (*error)(nil), err)
			// Verify it has a non-empty message
			assert.NotEmpty(t, err.Error())
		})
	}
}

func TestErrorCategories(t *testing.T) {
	// Group errors by their typical usage categories
	tests := []struct {
		name     string
		errors   []error
		category string
	}{
		{
			name:     "client errors (4xx)",
			errors:   []error{ErrProjectNotFound, ErrProjectSlugExists, ErrValidationFailed, ErrRevisionMismatch},
			category: "client",
		},
		{
			name:     "server errors (5xx)",
			errors:   []error{ErrInternal, ErrUnmarshal, ErrServiceUnavailable},
			category: "server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, err := range tt.errors {
				assert.NotNil(t, err)
				assert.NotEmpty(t, err.Error())

				// All errors should be distinct
				for _, otherErr := range tt.errors {
					if err != otherErr {
						assert.NotEqual(t, err.Error(), otherErr.Error())
					}
				}
			}
		})
	}
}