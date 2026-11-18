// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"fmt"
	"strings"
	"time"

	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
)

// InviteInfo holds the pending invite metadata for a user without an LFID.
type InviteInfo struct {
	UID       string     `json:"uid"`
	Email     string     `json:"email"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UserInfo represents user information including profile details.
type UserInfo struct {
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Username string      `json:"username"`
	Avatar   string      `json:"avatar"`
	Invite   *InviteInfo `json:"invite,omitempty"`
}

// FormationLead is a v1 contact, not necessarily an LFID user or an access-control role.
type FormationLead struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
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
	Funding                    string     `json:"funding"`
	FundingModel               []string   `json:"funding_model"`
	Industry                   []string   `json:"industry"`
	Technology                 []string   `json:"technology"`
	TechnicalActivityType      []string   `json:"technical_activity_type"`
	EntityDissolutionDate      *time.Time `json:"entity_dissolution_date"`
	EntityFormationDocumentURL string     `json:"entity_formation_document_url"`
	FormationDate              *time.Time `json:"formation_date"`
	AutojoinEnabled            bool       `json:"autojoin_enabled"`
	CharterURL                 string     `json:"charter_url"`
	LogoURL                    string     `json:"logo_url"`
	WebsiteURL                 string     `json:"website_url"`
	RepositoryURL              string     `json:"repository_url"`
	ContactEmail               string     `json:"contact_email"`
	CreatedAt                  *time.Time `json:"created_at"`
	UpdatedAt                  *time.Time `json:"updated_at"`
}

// ProjectSettings is the key-value store representation of a project settings.
type ProjectSettings struct {
	UID                     string         `json:"uid"`
	MissionStatement        string         `json:"mission_statement"`
	AnnouncementDate        *time.Time     `json:"announcement_date"`
	Auditors                []UserInfo     `json:"auditors"`
	Writers                 []UserInfo     `json:"writers"`
	MeetingCoordinators     []UserInfo     `json:"meeting_coordinators"`
	ExecutiveDirector       *UserInfo      `json:"executive_director,omitempty"`
	MentorshipProgramAdmins []UserInfo     `json:"mentorship_program_admins"`
	ProgramManager          *UserInfo      `json:"program_manager,omitempty"`
	OpportunityOwner        *UserInfo      `json:"opportunity_owner,omitempty"`
	FormationLead           *FormationLead `json:"formation_lead,omitempty"`
	CreatedAt               *time.Time     `json:"created_at"`
	UpdatedAt               *time.Time     `json:"updated_at"`
}

// Tags generates a consistent set of tags for the project base.
// IMPORTANT: If you modify this method, please update the Project Tags documentation in
// docs/indexer-contract.md to ensure consumers understand how to use these tags for searching.
func (p *ProjectBase) Tags() []string {
	if p == nil {
		return nil
	}

	var tags []string

	if p.Slug != "" {
		tag := fmt.Sprintf("project_slug:%s", p.Slug)
		tags = append(tags, tag)
	}

	return tags
}

// IndexingConfig generates an IndexingConfig for indexing this project.
// Every value is concrete: user text such as the project name must never be
// parsed as an indexer template (linuxfoundation/lfx-v2-indexer-service#89).
func (p *ProjectBase) IndexingConfig() *indexerTypes.IndexingConfig {
	if p == nil {
		return nil
	}

	return &indexerTypes.IndexingConfig{
		ObjectID:             p.UID,
		Public:               &p.Public,
		AccessCheckObject:    fmt.Sprintf("project:%s", p.UID),
		AccessCheckRelation:  "viewer",
		HistoryCheckObject:   fmt.Sprintf("project:%s", p.UID),
		HistoryCheckRelation: "writer_guard",
		SortName:             p.Name,
		NameAndAliases:       p.NameAndAliases(),
		ParentRefs:           p.ParentRefs(),
		Fulltext:             p.Fulltext(),
		Tags:                 p.Tags(),
	}
}

// ParentRefs generates the parent references for the project base.
func (p *ProjectBase) ParentRefs() []string {
	if p == nil || p.ParentUID == "" {
		return nil
	}
	return []string{fmt.Sprintf("project:%s", p.ParentUID)}
}

// NameAndAliases generates the searchable name and aliases for the project base.
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

// Fulltext generates the full-text searchable string for the project base.
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
// IMPORTANT: If you modify this method, please update the Project Settings Tags documentation in
// docs/indexer-contract.md to ensure consumers understand how to use these tags for searching.
func (p *ProjectSettings) Tags() []string {
	if p == nil {
		return nil
	}

	var tags []string

	if p.UID != "" {
		tags = append(tags, p.UID)
	}
	if p.MissionStatement != "" {
		tags = append(tags, p.MissionStatement)
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
		ObjectID:             p.UID,
		AccessCheckObject:    fmt.Sprintf("project:%s", projectUID),
		AccessCheckRelation:  "auditor_guard",
		HistoryCheckObject:   fmt.Sprintf("project:%s", projectUID),
		HistoryCheckRelation: "writer_guard",
		ParentRefs:           p.ParentRefs(),
		Tags:                 p.Tags(),
	}
}

// ParentRefs generates the parent references for the project settings.
func (p *ProjectSettings) ParentRefs() []string {
	if p == nil || p.UID == "" {
		return nil
	}
	return []string{fmt.Sprintf("project:%s", p.UID)}
}
