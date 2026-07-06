// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestSplitArgs(t *testing.T) {
	got := splitArgs([]string{"sync", "rename-project-slug", "--dry-run", "old", "new"}, 2)
	if len(got.Positionals) != 2 || got.Positionals[0] != "sync" || got.Positionals[1] != "rename-project-slug" {
		t.Fatalf("unexpected positionals: %#v", got.Positionals)
	}
	wantSubArgs := []string{"--dry-run", "old", "new"}
	if len(got.SubArgs) != len(wantSubArgs) {
		t.Fatalf("unexpected sub args: %#v", got.SubArgs)
	}
	for i := range wantSubArgs {
		if got.SubArgs[i] != wantSubArgs[i] {
			t.Fatalf("sub arg %d: want %q got %q", i, wantSubArgs[i], got.SubArgs[i])
		}
	}
}

func TestHasHelpFlag(t *testing.T) {
	if !hasHelpFlag([]string{"--dry-run", "--help"}) {
		t.Fatal("expected help flag to be detected")
	}
	if hasHelpFlag([]string{"--dry-run"}) {
		t.Fatal("did not expect help flag")
	}
}
