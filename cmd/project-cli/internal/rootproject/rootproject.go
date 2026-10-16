// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package rootproject names the hidden ROOT project that the CLI commands treat specially.
package rootproject

// Slug identifies the hidden ROOT project used for team permission
// assignment. It is created by scripts/root-project-setup (package main, so that
// script's own const is not importable) and must not appear in the search index.
// Compared case-sensitively on purpose: API slugs are validated against
// ^[a-z][a-z0-9_\-]*[a-z0-9]$ (api/project/v1/design/types.go), so "ROOT" cannot
// collide with a user slug, while a case-insensitive match would wrongly exclude a
// legitimate project slugged "root".
const Slug = "ROOT"
