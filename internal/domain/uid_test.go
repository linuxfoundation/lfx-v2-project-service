// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "testing"

func TestIsValidUID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"canonical lowercase", "00000000-0000-0000-0000-000000000001", true},
		{"canonical uppercase", "ABCDEF00-0000-0000-0000-000000000001", true},
		{"empty", "", false},
		{"slug-like", "folder-1", false},
		{"reservation key", "lookup/project-folders/" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"urn form", "urn:uuid:00000000-0000-0000-0000-000000000001", false},
		{"braced", "{00000000-0000-0000-0000-000000000001}", false},
		{"unhyphenated", "00000000000000000000000000000001", false},
		{"trailing slash", "00000000-0000-0000-0000-000000000001/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidUID(tt.in); got != tt.want {
				t.Errorf("IsValidUID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
