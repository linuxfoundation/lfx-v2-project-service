// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package misc

// StringPtr converts a string to a pointer to a string.
func StringPtr(s string) *string {
	return &s
}
