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

func (p *ProjectBase) TagsTemplatized() []string {
	var tags []string

	if p == nil {
		return nil
	}

	if p.Slug != "" {
		tags = append(tags, "project_slug:{{ slug }}")
	}

	return tags
}

// IndexingConfig generates an IndexingConfig for indexing this project.
func (p *ProjectBase) IndexingConfig() *indexerTypes.IndexingConfig {
	if p == nil {
		return nil
	}

	return &indexerTypes.IndexingConfig{
		ObjectID:             "{{ uid }}",
		Public:               &p.Public,
		AccessCheckObject:    fmt.Sprintf("project:%s", "{{ uid }}"),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", "{{ uid }}"),
		HistoryCheckRelation: "writer",
		SortName:             "{{ name }}",
		NameAndAliases:       p.NameAndAliasesTemplatized(),
		ParentRefs:           p.ParentRefsTemplatized(),
		Fulltext:             p.FulltextTemplatized(),
		Tags:                 p.TagsTemplatized(),
	}
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

// ParentRefsTemplatized generates a list of templatized parent references for the project base.
func (p *ProjectBase) ParentRefsTemplatized() []string {
	if p == nil {
		return nil
	}

	var parentRefs []string

	if p.ParentUID != "" {
		parentRefs = append(parentRefs, fmt.Sprintf("project:%s", "{{ parent_uid }}"))
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

// NameAndAliasesTemplatized generates a list of templatized name and aliases for the project base.
func (p *ProjectBase) NameAndAliasesTemplatized() []string {
	if p == nil {
		return nil
	}

	var nameAndAliases []string

	if p.Name != "" {
		nameAndAliases = append(nameAndAliases, "{{ name }}")
	}
	if p.Slug != "" {
		nameAndAliases = append(nameAndAliases, "{{ slug }}")
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

// FulltextTemplatized generates a fulltext string for the project base.
// This is used to index the project text that is full-text searchable.
func (p *ProjectBase) FulltextTemplatized() string {
	if p == nil {
		return ""
	}

	var fulltext []string

	if p.Name != "" {
		fulltext = append(fulltext, "{{ name }}")
	}
	if p.Slug != "" {
		fulltext = append(fulltext, "{{ slug }}")
	}
	if p.Description != "" {
		fulltext = append(fulltext, "{{ description }}")
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

// TagsTemplatized generates a list of templatized tags for the project settings.
func (p *ProjectSettings) TagsTemplatized() []string {
	if p == nil {
		return nil
	}

	var tags []string

	if p.UID != "" {
		tags = append(tags, "{{ uid }}")
	}

	if p.MissionStatement != "" {
		tags = append(tags, "{{ mission_statement }}")
	}

	return tags
}

// IndexingConfig generates an IndexingConfig for indexing this project settings.
// Note: Project settings use the project UID for access checks, not the settings UID.
func (p *ProjectSettings) IndexingConfig(projectUID string) *indexerTypes.IndexingConfig {
	if p == nil {
		return nil
	}

	return &indexerTypes.IndexingConfig{
		ObjectID:             "{{ uid }}",
		AccessCheckObject:    fmt.Sprintf("project:%s", projectUID),
		AccessCheckRelation:  "auditor",
		HistoryCheckObject:   fmt.Sprintf("project:%s", projectUID),
		HistoryCheckRelation: "writer",
		ParentRefs:           p.ParentRefsTemplatized(),
		Tags:                 p.TagsTemplatized(),
	}
}

// ParentRefs generates a list of parent references for the project settings.
// This is used to index the project settings as a child of its project.
func (p *ProjectSettings) ParentRefs() []string {
	if p == nil {
		return nil
	}

	var parentRefs []string

	if p.UID != "" {
		parentRefs = append(parentRefs, fmt.Sprintf("project:%s", p.UID))
	}

	return parentRefs
}

// ParentRefsTemplatized generates a list of templatized parent references for the project settings.
func (p *ProjectSettings) ParentRefsTemplatized() []string {
	if p == nil {
		return nil
	}

	var parentRefs []string

	if p.UID != "" {
		parentRefs = append(parentRefs, fmt.Sprintf("project:%s", "{{ uid }}"))
	}

	return parentRefs
}
