// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
)

// ProjectFolder represents an organizational folder for project links and documents.
type ProjectFolder struct {
	UID               string    `json:"uid"`
	ProjectUID        string    `json:"project_uid"`
	Name              string    `json:"name"`
	CreatedByUsername string    `json:"created_by_username,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BuildIndexKey returns a SHA-256 hash of projectUID|name for uniqueness enforcement.
func (f *ProjectFolder) BuildIndexKey(_ context.Context) string {
	data := fmt.Sprintf("%s|%s", f.ProjectUID, f.Name)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// IndexingConfig returns indexing configuration for the project folder.
func (f *ProjectFolder) IndexingConfig() *indexerTypes.IndexingConfig {
	if f == nil {
		return nil
	}
	return &indexerTypes.IndexingConfig{
		ObjectID:             f.UID,
		AccessCheckObject:    fmt.Sprintf("project:%s", f.ProjectUID),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", f.ProjectUID),
		HistoryCheckRelation: "auditor",
		SortName:             f.Name,
		ParentRefs:           []string{fmt.Sprintf("project:%s", f.ProjectUID)},
		Tags:                 f.Tags(),
	}
}

// Tags generates a consistent set of tags for the project folder.
func (f *ProjectFolder) Tags() []string {
	if f == nil {
		return nil
	}

	var tags []string

	if f.UID != "" {
		tags = append(tags, f.UID)
		tags = append(tags, fmt.Sprintf("project_folder_uid:%s", f.UID))
	}

	if f.ProjectUID != "" {
		tags = append(tags, fmt.Sprintf("project_uid:%s", f.ProjectUID))
	}

	return tags
}
