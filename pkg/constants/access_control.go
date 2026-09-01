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
	// RelationMarketingOps is the project relation granted to the per-project
	// marketing ops team object. It is a disjunct of both marketing_auditor and
	// campaign_manager in the OpenFGA model, so a single grant yields both.
	RelationMarketingOps = "marketing_ops"
	// MarketingOpsTeamPrefix is the per-project team object ID prefix used for
	// self-serve marketing ops membership. See marketingOpsTeamID in
	// internal/service/converters.go for the full ID derivation.
	MarketingOpsTeamPrefix = "marketing-ops-"
)
