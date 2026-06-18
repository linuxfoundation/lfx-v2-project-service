// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

// TestContactNormalization_15CharTo18Char guards the normalization step in
// ResolveOrCreateContact: Salesforce occasionally returns 15-char IDs from
// lookup fields; these must be expanded to canonical 18-char before storage.
func TestContactNormalization_15CharTo18Char(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		wantSFID string
	}{
		{
			name:     "15-char contact SFID expanded to 18-char",
			input:    "003B0000001ckSl",
			wantSFID: "003B0000001ckSlIAI",
		},
		{
			name:     "18-char contact SFID passes through unchanged",
			input:    "003B0000001ckSlIAI",
			wantSFID: "003B0000001ckSlIAI",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := sfuuid.Normalize18(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSFID, got)
		})
	}
}

// TestContactNormalization_ErrorIsNotValidation confirms that when
// ResolveOrCreateContact receives a malformed SFID from Salesforce, the
// resulting error is not a ValidationError (HTTP 400). The caller did not
// supply the ID — Salesforce did — so a 400 would be semantically wrong.
func TestContactNormalization_ErrorIsNotValidation(t *testing.T) {
	t.Parallel()

	_, sfuuidErr := sfuuid.Normalize18("not-a-valid-sfid")
	require.Error(t, sfuuidErr)

	// Mirrors what contact_repo.go does at each normalizeUID call site.
	wrapped := fmt.Errorf("malformed Contact SFID from Salesforce: %w", sfuuidErr)

	assert.False(t, pkgerrors.IsValidation(wrapped),
		"Salesforce-origin SFID errors must not surface as HTTP 400 ValidationError")
}
