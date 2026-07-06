// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"testing"
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
