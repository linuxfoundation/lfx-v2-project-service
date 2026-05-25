// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package b2borg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/sfuuid"
)

func TestResolver_UIDFromSFID(t *testing.T) {
	ctx := context.Background()
	r := NewResolver()

	tests := []struct {
		name    string
		sfid    string
		wantUID string
		wantErr bool
	}{
		{
			name:    "valid 18-char SFID returns deterministic UID",
			sfid:    "001B000000IqhSLIAZ",
			wantUID: "4c46585f-878c-8019-80e2-5632d301d19b",
		},
		{
			name:    "valid 15-char SFID returns deterministic UID",
			sfid:    "001B000000IqhSL",
			wantUID: "4c46585f-878c-8019-80e2-5632d301d19b",
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

func TestResolver_UIDFromSFID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	r := NewResolver()

	sfid := "001B000000IqhSLIAZ"
	uid, err := r.UIDFromSFID(ctx, sfid)
	require.NoError(t, err)

	back, err := sfuuid.ToSFID(uid)
	require.NoError(t, err)
	assert.Equal(t, sfid[:15], back, "round-trip SFID must match canonical 15-char form")
}
