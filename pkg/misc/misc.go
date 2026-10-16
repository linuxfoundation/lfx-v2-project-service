// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package misc

// StringPtr converts a string to a pointer to a string.
func StringPtr(s string) *string {
	return &s
}

// BoolPtr converts a bool to a pointer to a bool.
func BoolPtr(b bool) *bool {
	return &b
}

// ValueOrZero returns the value p points to, or the zero value of T if p is nil.
func ValueOrZero[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
