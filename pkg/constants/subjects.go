// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// NATS subjects used by the member-service publisher.
const (
	// IndexB2BOrgSubject is the NATS subject for indexing B2BOrg records.
	IndexB2BOrgSubject = "lfx.index.b2b_org"

	// IndexB2BOrgSettingsSubject is the NATS subject for indexing B2BOrgSettings records.
	IndexB2BOrgSettingsSubject = "lfx.index.b2b_org_settings"

	// IndexProjectMembershipSubject is the NATS subject for indexing ProjectMembership records.
	IndexProjectMembershipSubject = "lfx.index.project_membership"

	// IndexKeyContactSubject is the NATS subject for indexing KeyContact records.
	IndexKeyContactSubject = "lfx.index.key_contact"

	// FGASyncUpdateAccessSubject is the NATS subject for FGA update-access messages.
	FGASyncUpdateAccessSubject = "lfx.fga-sync.update_access"

	// FGASyncDeleteAccessSubject is the NATS subject for FGA delete-access messages.
	FGASyncDeleteAccessSubject = "lfx.fga-sync.delete_access"

	// AuthEmailToUsernameLookupSubject resolves a registered LFID username by primary email.
	// Request: plain-text email. Reply: plain-text username on success, JSON error envelope on miss.
	AuthEmailToUsernameLookupSubject = "lfx.auth-service.email_to_username"

	// IndexB2BOrgWorkspaceSubject is the NATS subject for indexing B2BOrgWorkspace records.
	IndexB2BOrgWorkspaceSubject = "lfx.index.b2b_org_workspace"
)
