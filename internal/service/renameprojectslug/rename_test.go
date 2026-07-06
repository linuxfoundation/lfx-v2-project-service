// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package renameprojectslug

import (
	"testing"
)

func TestBucketFieldsFor_knownBuckets(t *testing.T) {
	cases := []struct {
		bucket string
		field  string
	}{
		{"committee-members", "project_slug"},
		{"committees", "project_slug"},
		{"committee-settings", "project_slug"},
		{"projects", "slug"},
		{"project-settings", "project_slug"},
	}
	for _, c := range cases {
		fields := BucketFieldsFor(c.bucket)
		found := false
		for _, f := range fields {
			if f == c.field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BucketFieldsFor(%q): expected field %q, got %v", c.bucket, c.field, fields)
		}
	}
}

func TestBucketFieldsFor_unknownBucket(t *testing.T) {
	fields := BucketFieldsFor("some-unknown-bucket")
	if len(fields) != 1 || fields[0] != "project_slug" {
		t.Errorf("expected [project_slug] for unknown bucket, got %v", fields)
	}
}

func TestParseBuckets(t *testing.T) {
	got := ParseBuckets("committee-members, committees , committee-settings")
	want := []string{"committee-members", "committees", "committee-settings"}
	assertEqual(t, want, got)
}

func TestBuildOSQuery_containsOldSlug(t *testing.T) {
	const slug = "old-slug"
	q := buildOSQuery(slug)
	b, ok := q["bool"].(map[string]any)
	if !ok {
		t.Fatal("expected bool key in query")
	}
	should, ok := b["should"].([]any)
	if !ok {
		t.Fatal("expected should key in bool query")
	}
	if len(should) == 0 {
		t.Fatal("expected non-empty should clauses")
	}

	fields := map[string]bool{}
	for _, clause := range should {
		termClause, ok := clause.(map[string]any)
		if !ok {
			continue
		}
		term, ok := termClause["term"].(map[string]any)
		if !ok {
			continue
		}
		for k, v := range term {
			fields[k] = true
			if str, ok := v.(string); ok {
				if str != slug && str != "project:"+slug && str != "project_slug:"+slug {
					t.Errorf("unexpected term value for field %q: %q", k, str)
				}
			}
		}
	}

	for _, required := range []string{"data.project_slug", "object_ref", "parent_refs"} {
		if !fields[required] {
			t.Errorf("expected should clause for field %q, but it was missing", required)
		}
	}
}

func assertEqual[T comparable](t *testing.T, want, got []T) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("length mismatch: want %v, got %v", want, got)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("index %d: want %v, got %v", i, want[i], got[i])
		}
	}
}
