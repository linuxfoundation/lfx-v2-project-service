// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProjectRoleRemoved(t *testing.T) {
	tests := []struct {
		name        string
		data        ProjectRoleRemovedData
		wantSubject []string
		wantHTML    []string
		wantText    []string
		wantNoHTML  []string
	}{
		{
			name: "single role removed with inviter",
			data: ProjectRoleRemovedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Writer"},
				InviterName:   "Bob",
			},
			wantSubject: []string{"Demo Project"},
			wantHTML:    []string{"Alice", "Demo Project", "Writer", "Bob"},
			wantText:    []string{"Alice", "Demo Project", "Writer", "Bob"},
			wantNoHTML:  []string{"View Project"}, // no CTA button for removals
		},
		{
			name: "multiple roles removed",
			data: ProjectRoleRemovedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Writer", "Auditor"},
				InviterName:   "Bob",
			},
			wantHTML: []string{"Alice", "Demo Project", "Writer and Auditor"},
			wantText: []string{"Alice", "Demo Project", "Writer and Auditor"},
		},
		{
			name: "no inviter name",
			data: ProjectRoleRemovedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				OldRoles:      []string{"Auditor"},
			},
			wantSubject: []string{"Demo Project"},
			wantHTML:    []string{"Alice", "Demo Project"},
			wantText:    []string{"Alice", "Demo Project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, html, text, err := RenderProjectRoleRemoved(tt.data)
			require.NoError(t, err)

			for _, want := range tt.wantSubject {
				assert.Contains(t, subject, want)
			}
			for _, want := range tt.wantHTML {
				assert.Contains(t, html, want)
			}
			assert.True(t, strings.Contains(html, "<html"), "expected HTML output")
			for _, noWant := range tt.wantNoHTML {
				assert.NotContains(t, html, noWant)
			}
			for _, want := range tt.wantText {
				assert.Contains(t, text, want)
			}
			assert.False(t, strings.Contains(text, "<html"), "expected plain text output")
		})
	}
}
