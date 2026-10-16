// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	linkUID   = "00000000-0000-4000-8000-0000000000a1"
	folderUID = "00000000-0000-4000-8000-0000000000b1"
	docUID    = "00000000-0000-4000-8000-0000000000c1"
)

type fakeFileStore struct {
	missing map[string]bool
	errFor  string
}

func (f fakeFileStore) GetInfo(_ context.Context, name string, _ ...jetstream.GetObjectInfoOpt) (*jetstream.ObjectInfo, error) {
	if name == f.errFor {
		return nil, fmt.Errorf("boom")
	}
	if f.missing[name] {
		return nil, jetstream.ErrObjectNotFound
	}
	return &jetstream.ObjectInfo{}, nil
}

func rawJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// childBuckets builds link, folder and document buckets whose records and lookup keys agree.
func childBuckets(t *testing.T) (links, folders, docs *fakeKV) {
	t.Helper()
	link := &models.ProjectLink{UID: linkUID, ProjectUID: uid1, Name: "docs", URL: "https://example.com"}
	folder := &models.ProjectFolder{UID: folderUID, ProjectUID: uid1, Name: "legal"}
	doc := &models.ProjectDocument{UID: docUID, ProjectUID: uid1, Name: "charter", FileName: "charter.pdf"}

	links = &fakeKV{values: map[string]string{
		linkUID: rawJSON(t, link),
		fmt.Sprintf(constants.KVLookupLinkKey, uid1, linkUID): linkUID,
	}}
	folders = &fakeKV{values: map[string]string{
		folderUID: rawJSON(t, folder),
		fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(context.Background())): folderUID,
	}}
	docs = &fakeKV{values: map[string]string{
		docUID: rawJSON(t, doc),
		fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(context.Background())): docUID,
	}}
	return links, folders, docs
}

func TestRunChildren(t *testing.T) {
	linkLookup := fmt.Sprintf(constants.KVLookupLinkKey, uid1, linkUID)
	folderLookup := fmt.Sprintf(constants.KVLookupFolderPrefix, (&models.ProjectFolder{ProjectUID: uid1, Name: "legal"}).BuildIndexKey(context.Background()))
	docLookup := fmt.Sprintf(constants.KVLookupDocumentPrefix, (&models.ProjectDocument{ProjectUID: uid1, Name: "charter"}).BuildIndexKey(context.Background()))

	tests := []struct {
		name       string
		mutate     func(links, folders, docs *fakeKV)
		files      fileStore
		wantErr    string
		wantCounts map[string]int
	}{
		{name: "consistent buckets are clean", files: fakeFileStore{}},
		{name: "files are not checked when the store is nil"},
		{
			name:       "a link without its lookup key",
			mutate:     func(links, _, _ *fakeKV) { delete(links.values, linkLookup) },
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{"link" + kindMissingLookup: 1},
		},
		{
			name:       "a lookup key whose record is gone",
			mutate:     func(links, _, _ *fakeKV) { delete(links.values, linkUID) },
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{"link" + kindStaleLookup: 1},
		},
		{
			name:       "a folder lookup held by another folder",
			mutate:     func(_, folders, _ *fakeKV) { folders.values[folderLookup] = "someone-else" },
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{"folder" + kindWrongLookup: 1},
		},
		{
			name:       "a document without its lookup key",
			mutate:     func(_, _, docs *fakeKV) { delete(docs.values, docLookup) },
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{"document" + kindMissingLookup: 1},
		},
		{
			name:       "a document without its file",
			files:      fakeFileStore{missing: map[string]bool{docUID: true}},
			wantErr:    "found 1 problem(s)",
			wantCounts: map[string]int{kindDocumentNoFile: 1},
		},
		{
			name:       "an unreadable record",
			mutate:     func(links, _, _ *fakeKV) { links.values[linkUID] = "not json" },
			wantErr:    "found 2 problem(s)",
			wantCounts: map[string]int{"link" + kindChildUnreadable: 1, "link" + kindStaleLookup: 1},
		},
		{
			name:   "a lookup key of another resource is not this bucket's stale key",
			mutate: func(links, _, _ *fakeKV) { links.values["lookup/other/x"] = "v" },
		},
		{
			name:       "a key deleted mid-scan is ignored",
			mutate:     func(links, _, _ *fakeKV) { links.vanished = map[string]bool{"gone": true} },
			wantCounts: nil,
		},
		{name: "link bucket listing failure", mutate: func(links, _, _ *fakeKV) { links.listErr = fmt.Errorf("boom") }, wantErr: "scan link bucket"},
		{name: "folder bucket read failure", mutate: func(_, folders, _ *fakeKV) { folders.getErrFor = folderUID }, wantErr: "scan folder bucket"},
		{name: "document file read failure", files: fakeFileStore{errFor: docUID}, wantErr: "get document file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, folders, docs := childBuckets(t)
			if tt.mutate != nil {
				tt.mutate(links, folders, docs)
			}
			var out bytes.Buffer

			err := runChildren(context.Background(), links, folders, docs, tt.files, &out)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if out.Len() == 0 {
				return
			}
			var report childrenReport
			require.NoError(t, json.Unmarshal(out.Bytes(), &report))
			if tt.wantCounts == nil {
				assert.Empty(t, report.Findings)
				return
			}
			assert.Equal(t, tt.wantCounts, report.Counts)
		})
	}
}

func TestChildrenSubcommand_Run(t *testing.T) {
	runSubcommand(t, &childrenSubcommand{}, []runCase{
		{name: "rejects extra args", args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "fails at the NATS connect"},
		{name: "fails at the NATS connect with files skipped", args: []string{"--skip-files"}},
		{name: "help is not an error", args: []string{"-h"}, wantErr: "-"},
	})
}
