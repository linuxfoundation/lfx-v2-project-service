// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import "strings"

// sanitizeSubject strips ASCII control characters (0x00–0x1F and 0x7F) from s,
// preventing CRLF injection when the value is used as a MIME Subject header.
func sanitizeSubject(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}

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
