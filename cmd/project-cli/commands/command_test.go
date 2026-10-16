// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStats(t *testing.T) {
	before := time.Now()
	stats := NewStats()

	assert.False(t, stats.start.Before(before), "the clock starts when the stats are created")
	assert.Zero(t, stats.Total)
	assert.Zero(t, stats.Updated)
	assert.Zero(t, stats.Skipped)
	assert.Zero(t, stats.Failed)
	assert.False(t, stats.DryRun)
}

func TestStats_Log(t *testing.T) {
	capture := func(t *testing.T) *bytes.Buffer {
		t.Helper()
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })
		return &buf
	}

	t.Run("emits every counter under the command's name", func(t *testing.T) {
		buf := capture(t)
		stats := &Stats{Total: 10, Updated: 6, Skipped: 3, Failed: 1, DryRun: true, start: time.Now().Add(-2 * time.Second)}

		stats.Log(context.Background(), "sync example")

		var entry map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		assert.Equal(t, "sync example complete", entry["msg"])
		assert.EqualValues(t, 10, entry["total"])
		assert.EqualValues(t, 6, entry["updated"])
		assert.EqualValues(t, 3, entry["skipped"])
		assert.EqualValues(t, 1, entry["failed"])
		assert.Equal(t, true, entry["dry_run"])
		assert.GreaterOrEqual(t, entry["duration_ms"], float64(2000))
		assert.Greater(t, entry["rate_per_sec"], float64(0))
		assert.Less(t, entry["rate_per_sec"], float64(10), "rate is total over elapsed seconds")
	})

	t.Run("a zero duration reports a zero rate rather than dividing by zero", func(t *testing.T) {
		buf := capture(t)
		// A start in the future makes the elapsed time negative, which the
		// guard treats like zero.
		stats := &Stats{Total: 5, start: time.Now().Add(time.Hour)}

		stats.Log(context.Background(), "sync example")

		var entry map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		assert.EqualValues(t, 0, entry["rate_per_sec"])
	})
}

type namedSubcommand struct{ name string }

func (n namedSubcommand) Name() string                          { return n.name }
func (n namedSubcommand) Help() string                          { return "" }
func (n namedSubcommand) Run(context.Context, RunContext) error { return nil }

func TestIndexSubcommands(t *testing.T) {
	a, b := namedSubcommand{name: "a"}, namedSubcommand{name: "b"}

	index := IndexSubcommands(a, b)

	assert.Equal(t, map[string]Subcommand{"a": a, "b": b}, index)
	assert.Empty(t, IndexSubcommands())
}
