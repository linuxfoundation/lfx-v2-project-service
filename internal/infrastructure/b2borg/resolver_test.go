// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package b2borg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

func TestResolver_UIDFromSFID(t *testing.T) {
	ctx := context.Background()
	r := NewResolver()

	tests := []struct {
		name    string
		sfid    string
		wantUID string // canonical 18-char SFID
		wantErr bool
	}{
		{
			name:    "valid 18-char SFID returns same 18-char uid",
			sfid:    "001B000000IqhSLIAZ",
			wantUID: "001B000000IqhSLIAZ",
		},
		{
			name:    "valid 15-char SFID returns canonical 18-char uid",
			sfid:    "001B000000IqhSL",
			wantUID: "001B000000IqhSLIAZ",
		},
		{
			name:    "empty string returns not found",
			sfid:    "",
			wantErr: true,
		},
		{
			name:    "whitespace-only returns not found",
			sfid:    "   ",
			wantErr: true,
		},
		{
			name:    "too short returns not found",
			sfid:    "001B000",
			wantErr: true,
		},
		{
			name:    "invalid characters returns not found",
			sfid:    "not-an-sfid!!!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := r.UIDFromSFID(ctx, tt.sfid)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errs.IsNotFound(err), "expected NotFound error, got %T: %v", err, err)
				assert.Empty(t, uid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUID, uid)
		})
	}
}

// TestResolver_UIDFromSFID_RoundTrip verifies that the returned uid equals the
// canonical 18-char SFID (since uid IS the SFID in the current scheme).
func TestResolver_UIDFromSFID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	r := NewResolver()

	sfid18 := "001B000000IqhSLIAZ"
	uid, err := r.UIDFromSFID(ctx, sfid18)
	require.NoError(t, err)

	// uid must be the canonical 18-char SFID.
	assert.Equal(t, sfid18, uid, "uid must equal the canonical 18-char SFID")
	assert.Len(t, uid, 18, "uid must be exactly 18 chars")
}
