// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"fmt"
	"time"
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

	if p.UID != "" {
		tag := fmt.Sprintf("project_uid:%s", p.UID)
		tags = append(tags, tag)
	}

	if p.Slug != "" {
		tag := fmt.Sprintf("project_slug:%s", p.Slug)
		tags = append(tags, tag)
	}

	if p.Name != "" {
		tag := fmt.Sprintf("project_name:%s", p.Name)
		tags = append(tags, tag)
	}

	return tags
}

// Tags generates a consistent set of tags for the project settings.
// IMPORTANT: If you modify this method, please update the Project Tags documentation in the README.md
// to ensure consumers understand how to use these tags for searching.
func (p *ProjectSettings) Tags() []string {

	var tags []string

	if p == nil {
		return nil
	}

	if p.UID != "" {
		tag := fmt.Sprintf("project_uid:%s", p.UID)
		tags = append(tags, tag)
	}

	return tags
}
