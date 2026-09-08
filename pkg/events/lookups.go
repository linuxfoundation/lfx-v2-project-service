// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package events

import "time"

// The request and reply payloads for the project lookups served over NATS
// request/reply. They live here for the same reason the published event types
// do: a consuming service should import the definition rather than restate it.

// ProjectSettingsSummary is the reply to lfx.projects-api.get_settings.
//
// It is the grant roster and the announcement date, and deliberately not the
// whole settings record: the mission statement, the executive director, the
// program manager and the opportunity owner have no consumer on this subject,
// and a lookup that ships them makes every caller a holder of them.
//
// Writers and Auditors are always present, empty rather than null when the
// project has none configured, so a caller reading an empty roster does not
// have to tell it apart from a field that was omitted.
type ProjectSettingsSummary struct {
	UID              string     `json:"uid"`
	AnnouncementDate *time.Time `json:"announcement_date"`
	Writers          []UserInfo `json:"writers"`
	Auditors         []UserInfo `json:"auditors"`
}

// ProjectListRequest is the request body of lfx.projects-api.list_projects.
//
// The two filters are a union, not an intersection: the reply holds every
// project at one of Stages, plus every project named in UIDs whatever its
// stage. That is what lets a caller ask one question it otherwise has to ask
// twice — "the projects at the stages I care about, and also these specific
// ones, which I need the current stage of precisely because they may have left
// those stages."
//
// At least one filter must be set. An empty request is refused rather than
// answered with every project, because the caller that meant to send a filter
// and sent none would otherwise be served a full store scan that looks like a
// successful answer.
type ProjectListRequest struct {
	Stages []string `json:"stages,omitempty"`
	UIDs   []string `json:"uids,omitempty"`
}

// ProjectRef is one entry in the lfx.projects-api.list_projects reply: a
// project identified, with the fields a caller needs to decide what it is and
// what to do about it, and no more.
type ProjectRef struct {
	UID          string `json:"uid"`
	Slug         string `json:"slug"`
	IsFoundation bool   `json:"is_foundation"`
	ParentUID    string `json:"parent_uid"`
	Stage        string `json:"stage"`
}
