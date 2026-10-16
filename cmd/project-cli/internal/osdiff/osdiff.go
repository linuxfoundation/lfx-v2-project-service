// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package osdiff finds projects whose search documents are absent from the
// OpenSearch resources index. It is shared by the sync and inspect command
// groups so both report the same drift.
package osdiff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// ResourcesIndex is the shared OpenSearch index queried and written by the
// indexer service. There is no exported constant for it, so the literal lives here.
const ResourcesIndex = "resources"

const idChunkSize = 1000

// Document types in the resources index. The type is the indexer subject without
// its "lfx.index." prefix, and a document's _id is "<type>:<uid>".
const (
	TypeProject         = "project"
	TypeProjectSettings = "project_settings"
	TypeProjectLink     = "project_link"
	TypeProjectFolder   = "project_folder"
	TypeProjectDocument = "project_document"
)

// Doc names one search document the index is expected to hold.
type Doc struct {
	Type string
	UID  string
}

// ID is the document's _id in the resources index.
func (d Doc) ID() string { return d.Type + ":" + d.UID }

// MissingDocs returns the documents from docs that are absent from the resources
// index, in the order given.
func MissingDocs(ctx context.Context, client *opensearchgo.Client, docs []Doc) ([]Doc, error) {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc.ID()
	}

	found := make(map[string]struct{}, len(ids))
	for _, chunk := range chunkStrings(ids, idChunkSize) {
		hits, err := existingIDs(ctx, client, chunk)
		if err != nil {
			return nil, err
		}
		for id := range hits {
			found[id] = struct{}{}
		}
	}

	var missing []Doc
	for _, doc := range docs {
		if _, ok := found[doc.ID()]; !ok {
			missing = append(missing, doc)
		}
	}
	return missing, nil
}

// Missing records, for one project, whether its "project" and "project_settings"
// documents are absent. The two are resolved independently since a project can be
// missing one without the other.
type Missing struct {
	Project  bool
	Settings bool
}

// Any reports whether either document is absent.
func (m Missing) Any() bool { return m.Project || m.Settings }

// Diff resolves, for every project UID, whether its project and project_settings
// documents exist in the resources index.
func Diff(ctx context.Context, client *opensearchgo.Client, bases []*models.ProjectBase) (map[string]Missing, error) {
	docs := make([]Doc, 0, 2*len(bases))
	missing := make(map[string]Missing, len(bases))
	for _, base := range bases {
		docs = append(docs, Doc{TypeProject, base.UID}, Doc{TypeProjectSettings, base.UID})
		missing[base.UID] = Missing{}
	}

	absent, err := MissingDocs(ctx, client, docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range absent {
		m := missing[doc.UID]
		switch doc.Type {
		case TypeProject:
			m.Project = true
		case TypeProjectSettings:
			m.Settings = true
		}
		missing[doc.UID] = m
	}
	return missing, nil
}

func existingIDs(ctx context.Context, client *opensearchgo.Client, ids []string) (map[string]struct{}, error) {
	raw, err := json.Marshal(map[string]any{
		"size":    len(ids),
		"_source": false,
		"query": map[string]any{
			"ids": map[string]any{"values": ids},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	res, err := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex(ResourcesIndex),
		client.Search.WithBody(bytes.NewReader(raw)),
	)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("search error %s: %s", res.Status(), body)
	}

	var result struct {
		TimedOut bool `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	if result.TimedOut || result.Shards.Failed > 0 {
		return nil, fmt.Errorf("opensearch returned partial result (timed_out=%v, shards_failed=%d)",
			result.TimedOut, result.Shards.Failed)
	}

	found := make(map[string]struct{}, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		found[hit.ID] = struct{}{}
	}
	return found, nil
}

func chunkStrings(items []string, size int) [][]string {
	var chunks [][]string
	for len(items) > 0 {
		n := size
		if n > len(items) {
			n = len(items)
		}
		chunks = append(chunks, items[:n])
		items = items[n:]
	}
	return chunks
}
