// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		positionalLimit int
		wantPositionals []string
		wantSubArgs     []string
	}{
		{
			name:            "command and subcommand only",
			args:            []string{"sync", "rename-project-slug"},
			positionalLimit: 2,
			wantPositionals: []string{"sync", "rename-project-slug"},
			wantSubArgs:     nil,
		},
		{
			name:            "subcommand flags after subcommand",
			args:            []string{"sync", "rename-project-slug", "--dry-run", "--concurrency=10"},
			positionalLimit: 2,
			wantPositionals: []string{"sync", "rename-project-slug"},
			wantSubArgs:     []string{"--dry-run", "--concurrency=10"},
		},
		{
			name:            "slug positionals forwarded as sub args",
			args:            []string{"sync", "rename-project-slug", "--dry-run", "old-slug", "new-slug"},
			positionalLimit: 2,
			wantPositionals: []string{"sync", "rename-project-slug"},
			wantSubArgs:     []string{"--dry-run", "old-slug", "new-slug"},
		},
		{
			name:            "flag before first positional goes to SubArgs",
			args:            []string{"--unknown", "sync", "rename-project-slug"},
			positionalLimit: 2,
			wantPositionals: []string{"sync", "rename-project-slug"},
			wantSubArgs:     []string{"--unknown"},
		},
		{
			name:            "help flag after subcommand goes to SubArgs",
			args:            []string{"sync", "rename-project-slug", "--help"},
			positionalLimit: 2,
			wantPositionals: []string{"sync", "rename-project-slug"},
			wantSubArgs:     []string{"--help"},
		},
		{
			name:            "help flag before command goes to SubArgs",
			args:            []string{"--help"},
			positionalLimit: 2,
			wantPositionals: nil,
			wantSubArgs:     []string{"--help"},
		},
		{
			name:            "only one positional",
			args:            []string{"sync"},
			positionalLimit: 2,
			wantPositionals: []string{"sync"},
			wantSubArgs:     nil,
		},
		{
			name:            "no args",
			args:            []string{},
			positionalLimit: 2,
			wantPositionals: nil,
			wantSubArgs:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitArgs(tt.args, tt.positionalLimit)
			if !reflect.DeepEqual(got.Positionals, tt.wantPositionals) {
				t.Errorf("Positionals = %v, want %v", got.Positionals, tt.wantPositionals)
			}
			if !reflect.DeepEqual(got.SubArgs, tt.wantSubArgs) {
				t.Errorf("SubArgs = %v, want %v", got.SubArgs, tt.wantSubArgs)
			}
		})
	}
}

func TestHasHelpFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "long form --help",
			args: []string{"--help"},
			want: true,
		},
		{
			name: "short form -h",
			args: []string{"-h"},
			want: true,
		},
		{
			name: "help among other flags",
			args: []string{"--dry-run", "--help", "--old-slug=abc"},
			want: true,
		},
		{
			name: "no help flag",
			args: []string{"--dry-run", "--old-slug=abc"},
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			want: false,
		},
		{
			name: "nil args",
			args: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHelpFlag(tt.args); got != tt.want {
				t.Errorf("hasHelpFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
