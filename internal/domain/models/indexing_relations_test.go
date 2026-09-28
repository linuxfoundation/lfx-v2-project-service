// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"testing"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestIndexingConfig_CheckRelations pins the relation names every indexed document
// stores for the query service to check against OpenFGA. Read and write access are
// checked on the composed guard relations, which admit both the per-project grant
// and a global team holding the same capability; a bare grant name here would
// silently exclude the global teams from search results while the gateway still
// admitted them. `viewer` is deliberately not a guard: the model already composes
// the global relation into it.
func TestIndexingConfig_CheckRelations(t *testing.T) {
	tests := []struct {
		name        string
		config      *indexerTypes.IndexingConfig
		wantAccess  string
		wantHistory string
	}{
		{
			name:        "project",
			config:      (&ProjectBase{UID: "proj-001", Public: true}).IndexingConfig(),
			wantAccess:  "viewer",
			wantHistory: "writer_guard",
		},
		{
			name:        "project settings",
			config:      (&ProjectSettings{UID: "proj-001"}).IndexingConfig("proj-001"),
			wantAccess:  "auditor_guard",
			wantHistory: "writer_guard",
		},
		{
			name:        "folder",
			config:      (&ProjectFolder{UID: "folder-001", ProjectUID: "proj-001"}).IndexingConfig(),
			wantAccess:  "viewer",
			wantHistory: "auditor_guard",
		},
		{
			name:        "document",
			config:      (&ProjectDocument{UID: "doc-001", ProjectUID: "proj-001"}).IndexingConfig(),
			wantAccess:  "viewer",
			wantHistory: "auditor_guard",
		},
		{
			name:        "link",
			config:      (&ProjectLink{UID: "link-001", ProjectUID: "proj-001"}).IndexingConfig(),
			wantAccess:  "viewer",
			wantHistory: "auditor_guard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantAccess, tt.config.AccessCheckRelation)
			assert.Equal(t, tt.wantHistory, tt.config.HistoryCheckRelation)
		})
	}
}
