// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models

import "strings"

// CloneUserInfo returns a shallow copy of u, or nil when u is nil.
func CloneUserInfo(u *UserInfo) *UserInfo {
	if u == nil {
		return nil
	}
	cp := *u
	if u.Invite != nil {
		inv := *u.Invite
		cp.Invite = &inv
	}
	return &cp
}

// NormalizeLegacyAuditUsers populates CreatedBy/UpdatedBy from legacy flat username
// fields when reading older KV records. Idempotent for records already migrated.
func NormalizeLegacyAuditUsers(createdBy **UserInfo, updatedBy **UserInfo, legacyCreatedByUsername, legacyUploadedByUsername string) {
	if createdBy == nil || updatedBy == nil {
		return
	}
	if *createdBy == nil {
		legacy := strings.TrimSpace(legacyCreatedByUsername)
		if legacy == "" {
			legacy = strings.TrimSpace(legacyUploadedByUsername)
		}
		if legacy != "" {
			*createdBy = &UserInfo{Username: legacy}
		}
	}
	if *updatedBy == nil && *createdBy != nil {
		*updatedBy = CloneUserInfo(*createdBy)
	}
}

// AuditCreatorUsername returns the LFID username used for indexer tags.
func AuditCreatorUsername(createdBy *UserInfo) string {
	if createdBy == nil {
		return ""
	}
	return strings.TrimSpace(createdBy.Username)
}
