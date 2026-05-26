// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import "strings"

// joinRoles returns a grammatically-joined list of role names.
// [Writer] → "Writer"
// [Writer, Auditor] → "Writer and Auditor"
// [Writer, Auditor, Meeting Coordinator] → "Writer, Auditor, and Meeting Coordinator"
func joinRoles(roles []string) string {
	switch len(roles) {
	case 0:
		return ""
	case 1:
		return roles[0]
	case 2:
		return roles[0] + " and " + roles[1]
	default:
		return strings.Join(roles[:len(roles)-1], ", ") + ", and " + roles[len(roles)-1]
	}
}
