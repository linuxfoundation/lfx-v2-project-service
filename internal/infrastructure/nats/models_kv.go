// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package nats contains the models for the NATS messaging.
package nats

import "time"

// ProjectDB is the key-value store representation of a project.
type ProjectDB struct {
	UID         string    `json:"uid"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Public      bool      `json:"public"`
	ParentUID   string    `json:"parent_uid"`
	Auditors    []string  `json:"auditors"`
	Writers     []string  `json:"writers"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
