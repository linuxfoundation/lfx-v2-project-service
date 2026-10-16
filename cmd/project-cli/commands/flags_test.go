// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFlagSet(t *testing.T) {
	fs := NewFlagSet("demo", "sync demo [flags]")
	fs.Bool("update", false, "write changes")
	var out bytes.Buffer
	fs.SetOutput(&out)

	fs.Usage()

	assert.Contains(t, out.String(), "usage: project-cli sync demo [flags]")
	assert.Contains(t, out.String(), "flags:")
	assert.Contains(t, out.String(), "-update")
}

func TestParseFlags(t *testing.T) {
	newFS := func() (*bytes.Buffer, func(args []string) (bool, error), *bool) {
		fs := NewFlagSet("demo", "sync demo [flags]")
		var out bytes.Buffer
		fs.SetOutput(&out)
		update := fs.Bool("update", false, "write changes")
		return &out, func(args []string) (bool, error) { return ParseFlags(fs, args) }, update
	}

	t.Run("parses flags", func(t *testing.T) {
		_, parse, update := newFS()
		help, err := parse([]string{"--update"})
		require.NoError(t, err)
		assert.False(t, help)
		assert.True(t, *update)
	})

	t.Run("help prints usage and is not an error", func(t *testing.T) {
		out, parse, _ := newFS()
		for _, flagName := range []string{"-h", "--help"} {
			out.Reset()
			help, err := parse([]string{flagName})
			require.NoError(t, err)
			assert.True(t, help)
			assert.Contains(t, out.String(), "usage: project-cli sync demo")
		}
	})

	t.Run("an unknown flag is an error", func(t *testing.T) {
		_, parse, _ := newFS()
		help, err := parse([]string{"--nope"})
		require.ErrorContains(t, err, "flag provided but not defined")
		assert.False(t, help)
	})
}

func TestRejectExtraArgs(t *testing.T) {
	fs := NewFlagSet("demo", "sync demo [flags]")
	fs.SetOutput(&bytes.Buffer{})

	require.NoError(t, fs.Parse(nil))
	require.NoError(t, RejectExtraArgs(fs))

	require.NoError(t, fs.Parse([]string{"one", "two"}))
	require.EqualError(t, RejectExtraArgs(fs), "unexpected arguments: one two")
}
