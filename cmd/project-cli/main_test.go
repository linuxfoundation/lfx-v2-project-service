// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
)

// stubCommand and stubSubcommand let the registry-driven code paths run
// without touching NATS or OpenSearch.
type stubCommand struct {
	name string
	subs map[string]commands.Subcommand
}

func (c stubCommand) Name() string                                { return c.name }
func (c stubCommand) Help() string                                { return "stub command help" }
func (c stubCommand) Subcommands() map[string]commands.Subcommand { return c.subs }

type stubSubcommand struct {
	name string
	run  func(ctx context.Context, rc commands.RunContext) error
}

func (s stubSubcommand) Name() string { return s.name }
func (s stubSubcommand) Help() string { return "stub subcommand help" }
func (s stubSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	return s.run(ctx, rc)
}

// runWithArgs runs the CLI with the given arguments and returns what it wrote
// to stdout and stderr. run() reads os.Args and installs a new slog default,
// so both are restored afterwards.
func runWithArgs(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	prevArgs, prevStdout, prevStderr, prevLogger := os.Args, os.Stdout, os.Stderr, slog.Default()
	outR, outW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	errR, errW, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)

	os.Args = append([]string{"project-cli"}, args...)
	os.Stdout, os.Stderr = outW, errW
	defer func() {
		os.Args, os.Stdout, os.Stderr = prevArgs, prevStdout, prevStderr
		slog.SetDefault(prevLogger)
	}()

	err = run()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	outBytes, readErr := io.ReadAll(outR)
	require.NoError(t, readErr)
	errBytes, readErr := io.ReadAll(errR)
	require.NoError(t, readErr)
	return string(outBytes), string(errBytes), err
}

func TestRun(t *testing.T) {
	t.Setenv("JOB_RUN_ID", "run-123")

	t.Run("help flag with no command prints usage to stdout", func(t *testing.T) {
		stdout, _, err := runWithArgs(t, "--help")

		require.NoError(t, err)
		assert.Contains(t, stdout, "usage: project-cli <command> <subcommand>")
		assert.Contains(t, stdout, "sync")
		assert.Contains(t, stdout, "reindex-projects")
	})

	t.Run("help flag after only a command prints usage", func(t *testing.T) {
		stdout, _, err := runWithArgs(t, "sync", "-h")

		require.NoError(t, err)
		assert.Contains(t, stdout, "commands:")
	})

	t.Run("help for a subcommand prints that subcommand's flags", func(t *testing.T) {
		_, stderr, err := runWithArgs(t, "sync", "reindex-projects", "--help")

		require.NoError(t, err)
		assert.Contains(t, stderr, "usage: project-cli sync reindex-projects")
		assert.Contains(t, stderr, "-include-access")
	})

	t.Run("help for an unknown subcommand is an error", func(t *testing.T) {
		_, stderr, err := runWithArgs(t, "sync", "nope", "--help")

		require.EqualError(t, err, "unknown command: sync nope")
		assert.Contains(t, stderr, "unknown command: sync nope")
		assert.Contains(t, stderr, "usage: project-cli")
	})

	t.Run("help for an unknown command is an error", func(t *testing.T) {
		_, _, err := runWithArgs(t, "nope", "nope", "--help")

		require.EqualError(t, err, "unknown command: nope nope")
	})

	t.Run("no arguments prints usage and fails", func(t *testing.T) {
		_, stderr, err := runWithArgs(t)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: project-cli <command> <subcommand>")
		assert.Contains(t, stderr, "commands:")
	})

	t.Run("a command without a subcommand fails", func(t *testing.T) {
		_, _, err := runWithArgs(t, "sync")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("an unknown command fails", func(t *testing.T) {
		_, stderr, err := runWithArgs(t, "nope", "reindex-projects")

		require.EqualError(t, err, "unknown command: nope")
		assert.Contains(t, stderr, "commands:")
	})

	t.Run("an unknown subcommand fails", func(t *testing.T) {
		_, stderr, err := runWithArgs(t, "sync", "nope")

		require.EqualError(t, err, "unknown subcommand: sync nope")
		assert.Contains(t, stderr, "commands:")
	})

	t.Run("a known subcommand is dispatched and its error is returned", func(t *testing.T) {
		// --force without --project-uid is rejected by the subcommand itself,
		// before it connects to anything.
		_, _, err := runWithArgs(t, "sync", "reindex-projects", "--force")

		require.EqualError(t, err, "--force requires --project-uid")
	})
}

func TestResolveJobRunID(t *testing.T) {
	t.Run("JOB_RUN_ID wins", func(t *testing.T) {
		t.Setenv("JOB_RUN_ID", "job-1")
		t.Setenv("HOSTNAME", "pod-1")
		assert.Equal(t, "job-1", resolveJobRunID())
	})

	t.Run("surrounding whitespace is trimmed", func(t *testing.T) {
		t.Setenv("JOB_RUN_ID", "  job-1\n")
		t.Setenv("HOSTNAME", "pod-1")
		assert.Equal(t, "job-1", resolveJobRunID())
	})

	t.Run("a blank JOB_RUN_ID falls back to HOSTNAME", func(t *testing.T) {
		t.Setenv("JOB_RUN_ID", "   ")
		t.Setenv("HOSTNAME", "pod-1")
		assert.Equal(t, "pod-1", resolveJobRunID())
	})

	t.Run("a fresh UUID is generated when neither is set", func(t *testing.T) {
		t.Setenv("JOB_RUN_ID", "")
		t.Setenv("HOSTNAME", "")

		first, second := resolveJobRunID(), resolveJobRunID()

		_, err := uuid.Parse(first)
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})
}

func TestBuildRegistry(t *testing.T) {
	registry := buildRegistry()

	require.Contains(t, registry, "sync")
	assert.Equal(t, "sync", registry["sync"].Name())
	assert.Contains(t, registry["sync"].Subcommands(), "reindex-projects")
}

func TestPrintUsage(t *testing.T) {
	registry := map[string]commands.Command{
		"group": stubCommand{name: "group", subs: map[string]commands.Subcommand{
			"first": stubSubcommand{name: "first"},
		}},
	}

	var buf bytes.Buffer
	printUsage(&buf, registry)

	out := buf.String()
	assert.Contains(t, out, "usage: project-cli <command> <subcommand> [subcommand flags]")
	for _, envVar := range []string{"NATS_URL", "OPENSEARCH_URL", "LOG_LEVEL", "JOB_RUN_ID"} {
		assert.Contains(t, out, envVar)
	}
	assert.Contains(t, out, "cmd/project-cli/README.md")
	assert.Contains(t, out, "group")
	assert.Contains(t, out, "stub command help")
	assert.Contains(t, out, "first")
	assert.Contains(t, out, "stub subcommand help")
}
