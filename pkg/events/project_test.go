// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectSettingsUpdatedMessage(t *testing.T) {
	for _, origin := range []string{"", OriginV1SyncHelper, "future_source"} {
		t.Run("origin "+origin, func(t *testing.T) {
			message := ProjectSettingsUpdatedMessage{
				ProjectUID: "00000000-0000-0000-0000-000000000001",
				Actor:      Actor{Username: "test-user"}, Origin: origin,
			}
			data, err := json.Marshal(message)
			require.NoError(t, err)
			if origin == "" {
				assert.NotContains(t, string(data), `"origin"`)
			}
			var decoded ProjectSettingsUpdatedMessage
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, message, decoded)
			// An older subscriber ignores the additive field and retains attribution.
			var legacy struct {
				ProjectUID string `json:"project_uid"`
				Actor      Actor  `json:"actor"`
			}
			require.NoError(t, json.Unmarshal(data, &legacy))
			assert.Equal(t, message.Actor, legacy.Actor)
			assert.Equal(t, message.ProjectUID, legacy.ProjectUID)
		})
	}
	t.Run("legacy JSON", func(t *testing.T) {
		var message ProjectSettingsUpdatedMessage
		require.NoError(t, json.Unmarshal([]byte(`{"project_uid":"00000000-0000-0000-0000-000000000001","actor":{"username":"test-user"}}`), &message))
		assert.Empty(t, message.Origin)
		assert.Equal(t, "test-user", message.Actor.Username)
	})
}
