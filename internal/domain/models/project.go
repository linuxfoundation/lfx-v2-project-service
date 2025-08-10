// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import (
	"time"
)

// ProjectBase is the key-value store representation of a project base.
type ProjectBase struct {
	UID                        string     `json:"uid"`
	Slug                       string     `json:"slug"`
	Name                       string     `json:"name"`
	Description                string     `json:"description"`
	Public                     bool       `json:"public"`
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
	Auditors            []string   `json:"auditors"`
	Writers             []string   `json:"writers"`
	MeetingCoordinators []string   `json:"meeting_coordinators"`
	CreatedAt           *time.Time `json:"created_at"`
	UpdatedAt           *time.Time `json:"updated_at"`
}
