// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeSubject(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean input is unchanged",
			input: "You have been added as Writer on Demo Project",
			want:  "You have been added as Writer on Demo Project",
		},
		{
			name:  "CR is stripped",
			input: "Foo\rBar",
			want:  "FooBar",
		},
		{
			name:  "LF is stripped",
			input: "Foo\nBar",
			want:  "FooBar",
		},
		{
			name:  "CRLF sequence is stripped",
			input: "Evil\r\nBcc: attacker@example.com\r\nProject",
			want:  "EvilBcc: attacker@example.comProject",
		},
		{
			name:  "NUL is stripped",
			input: "Foo\x00Bar",
			want:  "FooBar",
		},
		{
			name:  "DEL (0x7F) is stripped",
			input: "Foo\x7FBar",
			want:  "FooBar",
		},
		{
			name:  "tab is stripped",
			input: "Foo\tBar",
			want:  "FooBar",
		},
		{
			name:  "empty input returns empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeSubject(tt.input))
		})
	}
}
