// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestResolveTarget(t *testing.T) {
	t.Setenv("TARGET", "nats")
	if got := resolveTarget(nil); got != "nats" {
		t.Fatalf("expected nats from env, got %q", got)
	}

	if got := resolveTarget([]string{"--target=opensearch"}); got != "opensearch" {
		t.Fatalf("expected opensearch from flag, got %q", got)
	}

	if got := resolveTarget([]string{"--dry-run=false", "--target=opensearch", "old", "new"}); got != "opensearch" {
		t.Fatalf("expected opensearch when flag follows other flags, got %q", got)
	}

	if got := resolveTarget([]string{"--dry-run=false", "--target", "nats", "old", "new"}); got != "nats" {
		t.Fatalf("expected nats from spaced flag form, got %q", got)
	}
}

func TestNeedsConnections(t *testing.T) {
	if !needsNATS("both") || !needsOpenSearch("both") {
		t.Fatal("both should require both clients")
	}
	if needsNATS("opensearch") || !needsOpenSearch("opensearch") {
		t.Fatal("opensearch target should only require OpenSearch")
	}
	if !needsNATS("nats") || needsOpenSearch("nats") {
		t.Fatal("nats target should only require NATS")
	}
}
