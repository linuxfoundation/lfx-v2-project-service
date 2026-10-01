// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "strings"

// CloneUserInfo returns a deep copy of u, or nil when u is nil.
// All pointer fields inside InviteInfo are copied by value so the caller
// and the original share no mutable state.
func CloneUserInfo(u *UserInfo) *UserInfo {
	if u == nil {
		return nil
	}
	cp := *u
	if u.Invite != nil {
		inv := *u.Invite
		if u.Invite.ExpiresAt != nil {
			t := *u.Invite.ExpiresAt
			inv.ExpiresAt = &t
		}
		cp.Invite = &inv
	}
	return &cp
}

// RedactAuditUser returns a display-only copy of u (name, username, avatar) for payloads
// readable at the viewer relation, such as indexer documents. Email and invite metadata are
// dropped so viewers, including the anonymous principal on public projects, never receive the
// audit user's contact details. Returns nil when u is nil.
func RedactAuditUser(u *UserInfo) *UserInfo {
	if u == nil {
		return nil
	}
	return &UserInfo{
		Name:     u.Name,
		Username: u.Username,
		Avatar:   u.Avatar,
	}
}

// NormalizeLegacyAuditUsers populates CreatedBy/UpdatedBy from legacy flat username
// fields when reading older KV records. Idempotent for records already migrated.
func NormalizeLegacyAuditUsers(createdBy, updatedBy *UserInfo, legacyCreatedByUsername, legacyUploadedByUsername string) (*UserInfo, *UserInfo) {
	if createdBy == nil {
		legacy := strings.TrimSpace(legacyCreatedByUsername)
		if legacy == "" {
			legacy = strings.TrimSpace(legacyUploadedByUsername)
		}
		if legacy != "" {
			createdBy = &UserInfo{Username: legacy}
		}
	}
	if updatedBy == nil && createdBy != nil {
		updatedBy = CloneUserInfo(createdBy)
	}
	return createdBy, updatedBy
}

// AuditCreatorUsername returns the LFID username used for indexer tags.
func AuditCreatorUsername(createdBy *UserInfo) string {
	if createdBy == nil {
		return ""
	}
	return strings.TrimSpace(createdBy.Username)
}

// AuditUserNeedsMigration reports whether a record still needs auth-service profile backfill.
func AuditUserNeedsMigration(u *UserInfo) bool {
	if u == nil {
		return false
	}
	if strings.TrimSpace(u.Username) == "" {
		return false
	}
	return strings.TrimSpace(u.Name) == ""
}
