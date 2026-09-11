// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewValidationError(t *testing.T) {
	tests := []struct {
		name         string
		reason       string
		wantSentinel bool // true → result must be the ErrValidationFailed sentinel itself
		wantMsg      string
	}{
		{
			name:         "empty reason returns sentinel directly",
			reason:       "",
			wantSentinel: true,
			wantMsg:      "validation failed",
		},
		{
			name:    "non-empty reason wraps sentinel with message",
			reason:  "name is required",
			wantMsg: "validation failed: name is required",
		},
		{
			name:    "reason with special characters is preserved",
			reason:  "field 'email' must contain '@'",
			wantMsg: "validation failed: field 'email' must contain '@'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewValidationError(tt.reason)

			// errors.Is must always match ErrValidationFailed regardless of wrapping.
			assert.ErrorIs(t, err, ErrValidationFailed)
			assert.Equal(t, tt.wantMsg, err.Error())

			if tt.wantSentinel {
				// Empty reason must return the sentinel itself, not a wrapped copy.
				// Use == to verify pointer/identity equality, not just value equality.
				assert.True(t, err == ErrValidationFailed,
					"empty reason should return the ErrValidationFailed sentinel directly")
			} else {
				// Non-empty reason must return a distinct wrapped error.
				assert.NotEqual(t, ErrValidationFailed, err,
					"non-empty reason should return a wrapped error, not the sentinel itself")
			}
		})
	}
}
