// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func projectRecord(uid, slug string) string {
	return fmt.Sprintf(`{"uid":%q,"slug":%q}`, uid, slug)
}

func TestRunSlugIndex(t *testing.T) {
	tests := []struct {
		name           string
		kv             *fakeKV
		wantErr        string
		wantCounts     map[string]int
		wantStats      [2]int // projects, slug entries
		wantDuplicates []duplicateSlugGroup
	}{
		{
			name: "consistent index is clean",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "alpha"), "slug/alpha": uid1,
				uid2: projectRecord(uid2, "beta"), "slug/beta": uid2,
			}},
			wantStats: [2]int{2, 2},
		},
		{
			name: "index entry for a deleted project is dangling",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "alpha"), "slug/alpha": uid1,
				"slug/gone": uid3,
			}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindDanglingIndex: 1},
			wantStats:  [2]int{1, 2},
		},
		{
			name: "index entry whose record has another slug is mismatched",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "renamed"), "slug/old": uid1, "slug/renamed": uid1,
			}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindMismatchedIndex: 1},
			wantStats:  [2]int{1, 2},
		},
		{
			name: "project without an index entry",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "alpha"),
			}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindMissingIndex: 1},
			wantStats:  [2]int{1, 0},
		},
		{
			name: "index entry that points at another project",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "alpha"), uid2: projectRecord(uid2, "beta"),
				"slug/alpha": uid2, "slug/beta": uid2,
			}},
			wantErr:    "found 2 problem(s)",
			wantCounts: map[string]int{kindWrongIndex: 1, kindMismatchedIndex: 1},
			wantStats:  [2]int{2, 2},
		},
		{
			name: "record that is not JSON",
			kv: &fakeKV{values: map[string]string{
				uid1: "not json",
			}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindUnreadable: 1},
			wantStats:  [2]int{0, 0},
		},
		{
			name: "duplicate records with different parents report both UIDs including the indexed winner",
			kv: &fakeKV{values: map[string]string{
				uid1: fmt.Sprintf(`{"uid":%q,"slug":"alpha","parent_uid":%q}`, uid1, uid3),
				uid2: projectRecord(uid2, "alpha"), "slug/alpha": uid2,
			}},
			wantErr:        "found 2 problem(s)",
			wantCounts:     map[string]int{kindDuplicateSlug: 1, kindWrongIndex: 1},
			wantStats:      [2]int{2, 1},
			wantDuplicates: []duplicateSlugGroup{{Slug: "alpha", ProjectUIDs: []string{uid1, uid2}, IndexedUID: uid2}},
		},
		{
			name: "duplicates are detected even without a slug index",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "alpha"), uid2: projectRecord(uid2, "alpha"),
			}},
			wantErr:        "found 3 problem(s)",
			wantCounts:     map[string]int{kindDuplicateSlug: 1, kindMissingIndex: 2},
			wantStats:      [2]int{2, 0},
			wantDuplicates: []duplicateSlugGroup{{Slug: "alpha", ProjectUIDs: []string{uid1, uid2}}},
		},
		{
			name: "duplicate groups and members are sorted and unrelated records are excluded",
			kv: &fakeKV{values: map[string]string{
				uid1: projectRecord(uid1, "beta"), uid2: projectRecord(uid2, "beta"), "slug/beta": uid1,
				uid3:                                   projectRecord(uid3, "alpha"),
				"00000000-0000-4000-8000-000000000004": projectRecord("00000000-0000-4000-8000-000000000004", "alpha"),
				"slug/alpha":                           uid3,
				"00000000-0000-4000-8000-000000000005": projectRecord("00000000-0000-4000-8000-000000000005", "gamma"),
				"slug/gamma":                           "00000000-0000-4000-8000-000000000005",
			}},
			wantErr:    "found 4 problem(s)",
			wantCounts: map[string]int{kindDuplicateSlug: 2, kindWrongIndex: 2},
			wantStats:  [2]int{5, 3},
			wantDuplicates: []duplicateSlugGroup{
				{Slug: "alpha", ProjectUIDs: []string{uid3, "00000000-0000-4000-8000-000000000004"}, IndexedUID: uid3},
				{Slug: "beta", ProjectUIDs: []string{uid1, uid2}, IndexedUID: uid1},
			},
		},
		{
			name: "a key deleted mid-scan is ignored",
			kv: &fakeKV{
				values:   map[string]string{uid1: projectRecord(uid1, "alpha"), "slug/alpha": uid1},
				vanished: map[string]bool{uid2: true, "slug/beta": true},
			},
			wantStats: [2]int{1, 1},
		},
		{name: "listing failure", kv: &fakeKV{listErr: fmt.Errorf("no stream")}, wantErr: "list project keys"},
		{
			name:    "read failure",
			kv:      &fakeKV{values: map[string]string{uid1: projectRecord(uid1, "alpha")}, getErrFor: uid1},
			wantErr: "get " + `"` + uid1 + `"`,
		},
		{
			name:    "slug read failure",
			kv:      &fakeKV{values: map[string]string{"slug/alpha": uid1}, getErrFor: "slug/alpha"},
			wantErr: "get \"slug/alpha\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runSlugIndex(context.Background(), tt.kv, &out)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantStats == [2]int{} && tt.wantErr != "" && out.Len() == 0 {
				return
			}

			var report slugIndexReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			assert.Equal(t, wantStatus(err), report.Status)
			assert.Equal(t, tt.wantStats[0], report.Projects)
			assert.Equal(t, tt.wantStats[1], report.SlugEntries)
			if tt.wantDuplicates == nil {
				assert.Empty(t, report.DuplicateSlugs)
			} else {
				assert.Equal(t, tt.wantDuplicates, report.DuplicateSlugs)
			}
			assert.NotNil(t, report.DuplicateSlugs, "a clean scan reports an empty array, not null")
			if tt.wantCounts == nil {
				assert.Empty(t, report.Counts)
				assert.Empty(t, report.Findings)
			} else {
				assert.Equal(t, tt.wantCounts, report.Counts)
			}
		})
	}
}

func TestGetValue(t *testing.T) {
	kv := &fakeKV{values: map[string]string{"a": "1"}, getErrFor: "bad"}

	raw, found, err := getValue(context.Background(), kv, "a")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "1", string(raw))

	_, found, err = getValue(context.Background(), kv, "missing")
	require.NoError(t, err)
	assert.False(t, found, "a missing key is not an error")

	_, _, err = getValue(context.Background(), kv, "bad")
	assert.Error(t, err)
}

func TestSortFindings(t *testing.T) {
	findings := []finding{
		{Kind: "b", Slug: "z", UID: "1"},
		{Kind: "a", Slug: "y", UID: "2"},
		{Kind: "a", Slug: "x", UID: "9"},
		{Kind: "a", Slug: "x", UID: "3"},
	}
	sortFindings(findings)
	assert.Equal(t, []finding{
		{Kind: "a", Slug: "x", UID: "3"},
		{Kind: "a", Slug: "x", UID: "9"},
		{Kind: "a", Slug: "y", UID: "2"},
		{Kind: "b", Slug: "z", UID: "1"},
	}, findings)
}

func TestSlugIndexSubcommand_Run(t *testing.T) {
	runSubcommand(t, &slugIndexSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "fails at the NATS connect"},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
