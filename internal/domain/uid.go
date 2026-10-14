// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "github.com/google/uuid"

// IsValidUID reports whether s is a canonical lowercase-or-uppercase hyphenated
// UUID (8-4-4-4-12). Resource UIDs double as NATS KV keys in buckets that also
// hold index and name-reservation keys, so anything else must never reach a
// KV lookup.
func IsValidUID(s string) bool {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return false
	}
	// uuid.Parse also accepts urn:uuid:, braces and unhyphenated forms.
	return len(s) == len(parsed.String())
}
