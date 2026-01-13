// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"fmt"
	"strings"
	"time"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
)

// UserInfo represents user information including profile details.
type UserInfo struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

// ProjectBase is the key-value store representation of a project base.
type ProjectBase struct {
	UID                        string     `json:"uid"`
	Slug                       string     `json:"slug"`
	Name                       string     `json:"name"`
	Description                string     `json:"description"`
	Public                     bool       `json:"public"`
	IsFoundation               bool       `json:"is_foundation"`
	ParentUID                  string     `json:"parent_uid"`
	Stage                      string     `json:"stage"`
	Category                   string     `json:"category"`
	LegalEntityType            string     `json:"legal_entity_type"`
	LegalEntityName            string     `json:"legal_entity_name"`
	LegalParentUID             string     `json:"legal_parent_uid"`
	FundingModel               []string   `json:"funding_model"`
	EntityDissolutionDate      *time.Time `json:"entity_dissolution_date"`
	EntityFormationDocumentURL string     `json:"entity_formation_document_url"`
	FormationDate              *time.Time `json:"formation_date"`
	AutojoinEnabled            bool       `json:"autojoin_enabled"`
	CharterURL                 string     `json:"charter_url"`
	LogoURL                    string     `json:"logo_url"`
	WebsiteURL                 string     `json:"website_url"`
	RepositoryURL              string     `json:"repository_url"`
	CreatedAt                  *time.Time `json:"created_at"`
	UpdatedAt                  *time.Time `json:"updated_at"`
}

// ProjectSettings is the key-value store representation of a project settings.
type ProjectSettings struct {
	UID                 string     `json:"uid"`
	MissionStatement    string     `json:"mission_statement"`
	AnnouncementDate    *time.Time `json:"announcement_date"`
	Auditors            []UserInfo `json:"auditors"`
	Writers             []UserInfo `json:"writers"`
	MeetingCoordinators []UserInfo `json:"meeting_coordinators"`
	CreatedAt           *time.Time `json:"created_at"`
	UpdatedAt           *time.Time `json:"updated_at"`
}

// Tags generates a consistent set of tags for the project base.
// IMPORTANT: If you modify this method, please update the Project Tags documentation in the README.md
// to ensure consumers understand how to use these tags for searching.
func (p *ProjectBase) Tags() []string {

	var tags []string

	if p == nil {
		return nil
	}

	if p.Slug != "" {
		tag := fmt.Sprintf("project_slug:%s", p.Slug)
		tags = append(tags, tag)
	}

	return tags
}

// IndexingConfig generates an IndexingConfig for indexing this project.
func (p *ProjectBase) IndexingConfig() *indexerTypes.IndexingConfig {
	if p == nil {
		return nil
	}

	config := indexerTypes.IndexingConfig{
		ObjectID:             p.UID,
		Public:               &p.Public,
		AccessCheckObject:    fmt.Sprintf("project:%s", p.UID),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", p.UID),
		HistoryCheckRelation: "writer",
		SortName:             p.Name,
		NameAndAliases:       p.NameAndAliases(),
		ParentRefs:           p.ParentRefs(),
		Fulltext:             p.Fulltext(),
		Tags:                 p.Tags(),
	}

	return &config
}

// ParentRefs generates a list of parent references for the project base.
// This is used to index the project as a child of its parent project.
func (p *ProjectBase) ParentRefs() []string {
	if p == nil {
		return nil
	}

	var parentRefs []string

	if p.ParentUID != "" {
		parentRefs = append(parentRefs, fmt.Sprintf("project:%s", p.ParentUID))
	}

	return parentRefs
}

// NameAndAliases generates a list of name and aliases for the project base.
// This is used to index the project with searchable names.
func (p *ProjectBase) NameAndAliases() []string {
	if p == nil {
		return nil
	}

	var nameAndAliases []string

	if p.Name != "" {
		nameAndAliases = append(nameAndAliases, p.Name)
	}
	if p.Slug != "" {
		nameAndAliases = append(nameAndAliases, p.Slug)
	}

	return nameAndAliases
}

// Fulltext generates a fulltext string for the project base.
// This is used to index the project text that is full-text searchable.
func (p *ProjectBase) Fulltext() string {
	if p == nil {
		return ""
	}

	var fulltext []string

	if p.Name != "" {
		fulltext = append(fulltext, p.Name)
	}
	if p.Slug != "" {
		fulltext = append(fulltext, p.Slug)
	}
	if p.Description != "" {
		fulltext = append(fulltext, p.Description)
	}

	return strings.Join(fulltext, " ")
}

// Tags generates a consistent set of tags for the project settings.
// IMPORTANT: If you modify this method, please update the Project Tags documentation in the README.md
// to ensure consumers understand how to use these tags for searching.
func (p *ProjectSettings) Tags() []string {
	if p == nil {
		return nil
	}
	return []string{}
}

// IndexingConfig generates an IndexingConfig for indexing this project settings.
// Note: Project settings use the project UID for access checks, not the settings UID.
func (p *ProjectSettings) IndexingConfig(projectUID string) *indexerTypes.IndexingConfig {
	if p == nil {
		return nil
	}

	return &indexerTypes.IndexingConfig{
		ObjectID:             p.UID,
		AccessCheckObject:    fmt.Sprintf("project:%s", projectUID),
		AccessCheckRelation:  "auditor",
		HistoryCheckObject:   fmt.Sprintf("project:%s", projectUID),
		HistoryCheckRelation: "writer",
		Tags:                 p.Tags(),
		ParentRefs:           p.ParentRefs(),
	}
}

func (p *ProjectSettings) ParentRefs() []string {
	if p == nil {
		return nil
	}
	return []string{fmt.Sprintf("project:%s", p.UID)}
}
