// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package natskvstore contains the models for the NATS key-value store.
package natskvstore

import "time"

// ProjectDB is the key-value store representation of a project.
type ProjectDB struct {
	UID         string    `json:"uid"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Managers    []string  `json:"managers"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
