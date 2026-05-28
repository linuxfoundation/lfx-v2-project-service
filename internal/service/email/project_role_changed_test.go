// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProjectRoleChanged(t *testing.T) {
	tests := []struct {
		name        string
		data        ProjectRoleChangedData
		wantSubject []string
		wantHTML    []string
		wantText    []string
	}{
		{
			name: "single role swap with inviter",
			data: ProjectRoleChangedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Writer"},
				NewRoles:      []string{"Auditor"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
				InviterName:   "Bob",
			},
			wantSubject: []string{"Bob", "Demo Project"},
			wantHTML:    []string{"Alice", "Demo Project", "Writer", "Auditor", "https://app.dev.lfx.dev/projects/demo-project", "Bob"},
			wantText:    []string{"Alice", "Demo Project", "Writer", "Auditor"},
		},
		{
			name: "role added to existing roles",
			data: ProjectRoleChangedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Writer"},
				NewRoles:      []string{"Writer", "Auditor"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
				InviterName:   "Bob",
			},
			wantHTML: []string{"Alice", "Demo Project", "Writer", "Auditor"},
			wantText: []string{"Alice", "Demo Project", "Writer", "Auditor"},
		},
		{
			name: "no inviter name — fallback subject",
			data: ProjectRoleChangedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Writer"},
				NewRoles:      []string{"Auditor"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
			},
			wantSubject: []string{"Demo Project"},
			wantHTML:    []string{"Alice", "Demo Project"},
			wantText:    []string{"Alice", "Demo Project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, html, text, err := RenderProjectRoleChanged(tt.data)
			require.NoError(t, err)

			for _, want := range tt.wantSubject {
				assert.Contains(t, subject, want)
			}
			for _, want := range tt.wantHTML {
				assert.Contains(t, html, want)
			}
			assert.True(t, strings.Contains(html, "<html"), "expected HTML output")
			for _, want := range tt.wantText {
				assert.Contains(t, text, want)
			}
			assert.False(t, strings.Contains(text, "<html"), "expected plain text output")
		})
	}
}
