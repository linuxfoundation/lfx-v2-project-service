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
	"strings"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// ResourcesIndex is the shared OpenSearch index queried and written by the
// indexer service. There is no exported constant for it, so the literal lives here.
const ResourcesIndex = "resources"

const idChunkSize = 1000

// Missing records, for one project, whether its "project" and "project_settings"
// documents are absent. The two are resolved independently since a project can be
// missing one without the other.
type Missing struct {
	Project  bool
	Settings bool
}

// Any reports whether either document is absent.
func (m Missing) Any() bool { return m.Project || m.Settings }

// Diff resolves, for every project UID, whether its documents exist in the
// resources index.
func Diff(ctx context.Context, client *opensearchgo.Client, bases []*models.ProjectBase) (map[string]Missing, error) {
	missing := make(map[string]Missing, len(bases))
	for _, base := range bases {
		missing[base.UID] = Missing{Project: true, Settings: true}
	}

	var ids []string
	for _, base := range bases {
		ids = append(ids, "project:"+base.UID, "project_settings:"+base.UID)
	}

	for _, chunk := range chunkStrings(ids, idChunkSize) {
		found, err := existingIDs(ctx, client, chunk)
		if err != nil {
			return nil, err
		}
		for id := range found {
			uid, docType, ok := splitResourceID(id)
			if !ok {
				continue
			}
			m := missing[uid]
			switch docType {
			case "project":
				m.Project = false
			case "project_settings":
				m.Settings = false
			}
			missing[uid] = m
		}
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

// splitResourceID splits a resources index _id of the form "<object_type>:<uid>"
// into its UID and object type.
func splitResourceID(id string) (uid, docType string, ok bool) {
	for _, prefix := range []string{"project_settings:", "project:"} {
		if len(id) > len(prefix) && strings.HasPrefix(id, prefix) {
			return id[len(prefix):], strings.TrimSuffix(prefix, ":"), true
		}
	}
	return "", "", false
}
