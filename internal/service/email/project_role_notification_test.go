// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProjectRoleNotification(t *testing.T) {
	tests := []struct {
		name        string
		data        ProjectRoleNotificationData
		wantSubject []string
		wantNotSubj []string
		wantHTML    []string
		wantText    []string
	}{
		{
			name: "single role with inviter",
			data: ProjectRoleNotificationData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				Roles:         []string{"Writer"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
				InviterName:   "Bob",
			},
			wantSubject: []string{"Writer", "Demo Project", "Bob"},
			wantHTML:    []string{"Alice", "Demo Project", "Writer", "https://app.dev.lfx.dev/projects/demo-project", "Bob"},
			wantText:    []string{"Alice", "Demo Project", "Writer", "https://app.dev.lfx.dev/projects/demo-project", "Bob"},
		},
		{
			name: "multiple roles with inviter",
			data: ProjectRoleNotificationData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				Roles:         []string{"Writer", "Auditor"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
				InviterName:   "Bob",
			},
			wantSubject: []string{"Writer and Auditor", "Demo Project", "Bob"},
			wantHTML:    []string{"Alice", "Demo Project", "Writer and Auditor", "https://app.dev.lfx.dev/projects/demo-project", "Bob"},
			wantText:    []string{"Alice", "Demo Project", "Writer and Auditor"},
		},
		{
			name: "no inviter name — fallback subject",
			data: ProjectRoleNotificationData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				Roles:         []string{"Auditor"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
			},
			wantSubject: []string{"Auditor", "Demo Project"},
			wantHTML:    []string{"Alice", "Demo Project", "Auditor"},
			wantText:    []string{"Alice", "Demo Project", "Auditor"},
		},
		{
			name: "manage role — capability list in body",
			data: ProjectRoleNotificationData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				Roles:         []string{"Manage"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
				InviterName:   "Bob",
			},
			wantSubject: []string{"Manage", "Demo Project", "Bob"},
			wantHTML:    []string{"With the", "Manage", "role, you can", "Create &amp; update subprojects", "Update project settings"},
			wantText:    []string{"With the Manage role, you can", "- Create & update subprojects", "- Update project settings"},
		},
		{
			name: "view role — capability list in body",
			data: ProjectRoleNotificationData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				Roles:         []string{"View"},
				ProjectURL:    "https://app.dev.lfx.dev/projects/demo-project",
			},
			wantSubject: []string{"View", "Demo Project"},
			wantHTML:    []string{"With the", "View", "role, you can", "View project settings"},
			wantText:    []string{"With the View role, you can", "- View project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, html, text, err := RenderProjectRoleNotification(tt.data)
			require.NoError(t, err)

			for _, want := range tt.wantSubject {
				assert.Contains(t, subject, want)
			}
			for _, notWant := range tt.wantNotSubj {
				assert.NotContains(t, subject, notWant)
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
