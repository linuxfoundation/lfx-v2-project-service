// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDomainAliases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "empty string returns nil",
			raw:  "",
			want: nil,
		},
		{
			name: "single domain",
			raw:  "lf.org",
			want: []string{"lf.org"},
		},
		{
			name: "comma separated",
			raw:  "lf.org, thelinuxfoundation.org",
			want: []string{"lf.org", "thelinuxfoundation.org"},
		},
		{
			name: "comma no space",
			raw:  "lf.org,thelinuxfoundation.org",
			want: []string{"lf.org", "thelinuxfoundation.org"},
		},
		{
			name: "LF newline separated",
			raw:  "lf.org\nthelinuxfoundation.org",
			want: []string{"lf.org", "thelinuxfoundation.org"},
		},
		{
			name: "CRLF newline separated",
			raw:  "lf.org\r\nthelinuxfoundation.org",
			want: []string{"lf.org", "thelinuxfoundation.org"},
		},
		{
			name: "CR only newline separated",
			raw:  "lf.org\rthelinuxfoundation.org",
			want: []string{"lf.org", "thelinuxfoundation.org"},
		},
		{
			name: "mixed comma and CRLF",
			raw:  "a.com\r\nb.com,c.com",
			want: []string{"a.com", "b.com", "c.com"},
		},
		{
			name: "Salesforce merge artifact — recovers both valid domains (prod PwC bug)",
			raw:  "pwc.com.pg\r\n\r\n--- Merged Data:\r\n\r\npwc.ai",
			want: []string{"pwc.com.pg", "pwc.ai"},
		},
		{
			name: "merge artifact with longer separator line",
			raw:  "a.com\r\n--- Merged Data: 2024-01-01\r\nb.com",
			want: []string{"a.com", "b.com"},
		},
		{
			name: "dot-less token rejected",
			raw:  "Merged",
			want: []string{},
		},
		{
			name: "valid domain with dot-less token mixed in",
			raw:  "a.com, Merged, b.com",
			want: []string{"a.com", "b.com"},
		},
		{
			name: "all separators and artifacts, no valid domains",
			raw:  "---\r\n\r\n---\r\n",
			want: []string{},
		},
		{
			name: "whitespace only tokens skipped",
			raw:  "a.com,   ,b.com",
			want: []string{"a.com", "b.com"},
		},
		{
			name: "trims surrounding whitespace from domains",
			raw:  "  a.com  ,  b.com  ",
			want: []string{"a.com", "b.com"},
		},
		{
			name: "URL with slash rejected",
			raw:  "https://a.com",
			want: []string{},
		},
		{
			name: "trailing newlines produce no empty entries",
			raw:  "a.com\r\n\r\n",
			want: []string{"a.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseDomainAliases(ctx, "test-sfid", tc.raw)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{"valid bare domain", "example.com", "example.com", true},
		{"valid subdomain", "sub.example.com", "sub.example.com", true},
		{"single label rejected", "example", "", false},
		{"single label merge artifact", "Merged", "", false},
		{"contains slash rejected", "example.com/path", "", false},
		{"contains space rejected", "example .com", "", false},
		{"empty string rejected", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := normalizeDomain(tc.input)
			assert.Equal(t, tc.wantOK, ok, "ok mismatch")
			assert.Equal(t, tc.want, got, "value mismatch")
		})
	}
}
