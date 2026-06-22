// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package constants contains the constants for the project service.
package constants

const (
	// AccessCheckSubject is the subject used for access control checks
	// The subject is of the form: lfx.access_check.request
	AccessCheckSubject = "lfx.access_check.request"
	// AnonymousPrincipal is the identifier for anonymous users
	AnonymousPrincipal = `_anonymous`

	// RootProjectSlug is the slug of the platform ROOT project used for global team assignments.
	RootProjectSlug = "ROOT"

	// RelationMarketingOps is the OpenFGA relation for the global Marketing Ops team on ROOT.
	RelationMarketingOps = "marketing_ops"
)
