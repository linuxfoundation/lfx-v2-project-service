// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()

	assert.Equal(t, "sync", cmd.Name())
	assert.NotEmpty(t, cmd.Help())

	subs := cmd.Subcommands()
	require.NotEmpty(t, subs)
	for key, sub := range subs {
		assert.Equal(t, key, sub.Name(), "the registry key must match the subcommand's own name")
		assert.NotEmpty(t, sub.Help(), "%s needs help text", key)
	}
	assert.ElementsMatch(t,
		[]string{"rename-project-slug", "document-audit-users", "reindex-projects", "verify-global-grants"},
		keysOf(subs))
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
