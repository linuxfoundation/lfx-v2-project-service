// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/env"
)

func TestResolveSlugs_fromFlags(t *testing.T) {
	old, new, err := resolveSlugs("old-slug", "new-slug", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if old != "old-slug" || new != "new-slug" {
		t.Fatalf("got %q %q", old, new)
	}
}

func TestResolveSlugs_fromPositionals(t *testing.T) {
	old, new, err := resolveSlugs("", "", []string{"old-slug", "new-slug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if old != "old-slug" || new != "new-slug" {
		t.Fatalf("got %q %q", old, new)
	}
}

func TestResolveSlugs_rejectsMixedInput(t *testing.T) {
	_, _, err := resolveSlugs("old-slug", "", []string{"new-slug"})
	if err == nil {
		t.Fatal("expected error when mixing flags and positional args")
	}
}

func TestResolveSlugs_requiresBothSlugs(t *testing.T) {
	_, _, err := resolveSlugs("old-slug", "", nil)
	if err == nil {
		t.Fatal("expected error when new slug is missing")
	}
}

func TestRenameProjectSlugSubcommand_Run(t *testing.T) {
	s := &renameProjectSlugSubcommand{}
	assert.Equal(t, "rename-project-slug", s.Name())
	assert.NotEmpty(t, s.Help())

	// Slug and dry-run defaults come from the environment, so pin them.
	t.Setenv("OLD_SLUG", "")
	t.Setenv("NEW_SLUG", "")
	t.Setenv("DRY_RUN", "")

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "help is not an error", args: []string{"-h"}},
		{name: "no slugs", wantErr: "usage: project-cli sync rename-project-slug"},
		{name: "only one positional slug", args: []string{"old-slug"}, wantErr: "expected exactly 2 positional args"},
		{name: "flags and positionals together", args: []string{"--old-slug=a", "b", "c"}, wantErr: "not both"},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "flag provided but not defined"},
		{name: "contradictory write mode is rejected before connecting", args: []string{"--update", "--dry-run", "a", "b"}, wantErr: "conflicting write mode"},
		{name: "valid slugs proceed to connect to NATS", args: []string{"old-slug", "new-slug"}, wantErr: "failed to connect to NATS"},
		{name: "slugs from flags proceed to connect to NATS", args: []string{"--old-slug=old-slug", "--new-slug=new-slug"}, wantErr: "failed to connect to NATS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Run(context.Background(), commands.RunContext{Args: tt.args, NATSConfig: unreachableNATS()})
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	t.Run("slugs can come from the environment", func(t *testing.T) {
		t.Setenv("OLD_SLUG", "old-slug")
		t.Setenv("NEW_SLUG", "new-slug")

		err := s.Run(context.Background(), commands.RunContext{NATSConfig: unreachableNATS()})

		require.ErrorContains(t, err, "failed to connect to NATS")
	})
}

func TestResolveWriteMode(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		env       map[string]string
		wantApply bool
		wantErr   string
	}{
		{name: "previews by default"},
		{name: "--update writes", args: []string{"--update"}, wantApply: true},
		{name: "--update=false previews", args: []string{"--update=false"}},
		{name: "deprecated --dry-run=false writes", args: []string{"--dry-run=false"}, wantApply: true},
		{name: "deprecated --dry-run=true previews", args: []string{"--dry-run=true"}},
		{name: "bare deprecated --dry-run previews", args: []string{"--dry-run"}},
		{name: "consistent flags are accepted", args: []string{"--update", "--dry-run=false"}, wantApply: true},
		{name: "--update with --dry-run is contradictory", args: []string{"--update", "--dry-run"}, wantErr: "conflicting write mode"},
		{name: "--update=false with --dry-run=false is contradictory", args: []string{"--update=false", "--dry-run=false"}, wantErr: "conflicting write mode"},
		{name: "UPDATE env writes", env: map[string]string{"UPDATE": "true"}, wantApply: true},
		{name: "deprecated DRY_RUN=false env writes", env: map[string]string{"DRY_RUN": "false"}, wantApply: true},
		{name: "deprecated DRY_RUN=true env previews", env: map[string]string{"DRY_RUN": "true"}},
		{name: "a flag overrides the env", args: []string{"--dry-run=true"}, env: map[string]string{"DRY_RUN": "false"}},
		{name: "UPDATE env with the deprecated flag is checked for contradiction", args: []string{"--dry-run"}, env: map[string]string{"UPDATE": "true"}, wantErr: "conflicting write mode"},
		{name: "an unparseable DRY_RUN fails safe to a preview", env: map[string]string{"DRY_RUN": "maybe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("UPDATE", "")
			t.Setenv("DRY_RUN", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			fs := commands.NewFlagSet("rename-project-slug", "sync rename-project-slug")
			update := fs.Bool("update", env.GetBool("UPDATE", false), "")
			dryRun := fs.Bool("dry-run", env.GetBool("DRY_RUN", true), "")
			fs.SetOutput(io.Discard)
			require.NoError(t, fs.Parse(tt.args))

			apply, err := resolveWriteMode(context.Background(), fs, *update, *dryRun)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantApply, apply)
		})
	}
}

func TestWithSlugContext(t *testing.T) {
	// The attributes ride on the context and only surface through the handler
	// installed by log.InitStructureLogConfig, which writes to os.Stdout.
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	prevStdout, prevDefault := os.Stdout, slog.Default()
	prevLogWriter, prevLogFlags := stdlog.Writer(), stdlog.Flags()
	os.Stdout = writer
	// The probe below is info level, so do not inherit a quieter LOG_LEVEL.
	t.Setenv("LOG_LEVEL", "info")
	log.InitStructureLogConfig()
	t.Cleanup(func() {
		os.Stdout = prevStdout
		slog.SetDefault(prevDefault)
		stdlog.SetOutput(prevLogWriter)
		stdlog.SetFlags(prevLogFlags)
	})

	ctx := withSlugContext(context.Background(), "old-slug", "new-slug", true)
	slog.InfoContext(ctx, "probe")

	require.NoError(t, writer.Close())
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &entry), string(raw))
	assert.Equal(t, "old-slug", entry["old_slug"])
	assert.Equal(t, "new-slug", entry["new_slug"])
	assert.Equal(t, true, entry["dry_run"])
}
