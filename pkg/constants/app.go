// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// Constants for the application.
const (
	// ErrKey is the key for the error in the context.
	ErrKey = "error"

	// RootProjectSlug identifies the hidden ROOT project used for team permission
	// assignment. It is created by scripts/root-project-setup and must not appear
	// in the search index. Compared case-sensitively on purpose: API slugs are
	// validated against ^[a-z][a-z0-9_\-]*[a-z0-9]$ (api/project/v1/design/types.go),
	// so "ROOT" cannot collide with a user slug, while a case-insensitive match
	// would wrongly exclude a legitimate project slugged "root".
	RootProjectSlug = "ROOT"
)
