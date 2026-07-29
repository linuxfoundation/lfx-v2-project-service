// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
)

// ProjectLink represents a URL reference associated with a project.
type ProjectLink struct {
	UID         string    `json:"uid"`
	ProjectUID  string    `json:"project_uid"`
	FolderUID   *string   `json:"folder_uid,omitempty"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Description string    `json:"description,omitempty"`
	CreatedBy   *UserInfo `json:"created_by,omitempty"`
	UpdatedBy   *UserInfo `json:"updated_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type projectLinkJSON struct {
	UID               string    `json:"uid"`
	ProjectUID        string    `json:"project_uid"`
	FolderUID         *string   `json:"folder_uid,omitempty"`
	Name              string    `json:"name"`
	URL               string    `json:"url"`
	Description       string    `json:"description,omitempty"`
	CreatedBy         *UserInfo `json:"created_by,omitempty"`
	UpdatedBy         *UserInfo `json:"updated_by,omitempty"`
	CreatedByUsername string    `json:"created_by_username,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (l *ProjectLink) UnmarshalJSON(data []byte) error {
	var raw projectLinkJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	l.UID = raw.UID
	l.ProjectUID = raw.ProjectUID
	l.FolderUID = raw.FolderUID
	l.Name = raw.Name
	l.URL = raw.URL
	l.Description = raw.Description
	l.CreatedBy = raw.CreatedBy
	l.UpdatedBy = raw.UpdatedBy
	l.CreatedAt = raw.CreatedAt
	l.UpdatedAt = raw.UpdatedAt
	l.CreatedBy, l.UpdatedBy = NormalizeLegacyAuditUsers(l.CreatedBy, l.UpdatedBy, raw.CreatedByUsername, "")
	return nil
}

func (l *ProjectLink) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("null"), nil
	}
	return json.Marshal(&projectLinkJSON{
		UID:         l.UID,
		ProjectUID:  l.ProjectUID,
		FolderUID:   l.FolderUID,
		Name:        l.Name,
		URL:         l.URL,
		Description: l.Description,
		CreatedBy:   l.CreatedBy,
		UpdatedBy:   l.UpdatedBy,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
	})
}

// BuildIndexKey returns a SHA-256 hash of projectUID|uid used by the indexer service.
func (l *ProjectLink) BuildIndexKey(_ context.Context) string {
	if l == nil {
		return ""
	}
	data := fmt.Sprintf("%s|%s", l.ProjectUID, l.UID)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// IndexingConfig returns indexing configuration for the project link.
func (l *ProjectLink) IndexingConfig() *indexerTypes.IndexingConfig {
	if l == nil {
		return nil
	}
	return &indexerTypes.IndexingConfig{
		ObjectID:             l.UID,
		AccessCheckObject:    fmt.Sprintf("project:%s", l.ProjectUID),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", l.ProjectUID),
		HistoryCheckRelation: "auditor",
		SortName:             l.Name,
		ParentRefs:           []string{fmt.Sprintf("project:%s", l.ProjectUID)},
		Tags:                 l.Tags(),
	}
}

// Tags generates a consistent set of tags for the project link.
func (l *ProjectLink) Tags() []string {
	if l == nil {
		return nil
	}

	var tags []string

	if l.UID != "" {
		tags = append(tags, l.UID)
		tags = append(tags, fmt.Sprintf("project_link_uid:%s", l.UID))
	}

	if l.ProjectUID != "" {
		tags = append(tags, fmt.Sprintf("project_uid:%s", l.ProjectUID))
	}

	if l.FolderUID != nil && *l.FolderUID != "" {
		tags = append(tags, fmt.Sprintf("folder_uid:%s", *l.FolderUID))
	}

	if username := AuditCreatorUsername(l.CreatedBy); username != "" {
		tags = append(tags, fmt.Sprintf("uploaded_by:%s", username))
	}

	return tags
}
