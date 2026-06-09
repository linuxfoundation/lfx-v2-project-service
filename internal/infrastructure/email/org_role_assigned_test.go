// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email_test

import (
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	infraemail "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/email"
)

func TestRenderOrgRoleAssigned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		role           string
		wantSubjectSfx string
		wantRoleLabel  string
	}{
		{
			name:           "writer gets Company Administrator subject and administrator label",
			role:           model.B2BOrgRoleWriter,
			wantSubjectSfx: "Company Administrator",
			wantRoleLabel:  "an administrator",
		},
		{
			name:           "auditor gets viewer subject and viewer label",
			role:           model.B2BOrgRoleAuditor,
			wantSubjectSfx: "viewer",
			wantRoleLabel:  "a viewer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const orgName = "Acme Corp"
			const platformURL = "https://selfserve.example.com"

			subject, htmlBody, textBody, err := infraemail.RenderOrgRoleAssigned(orgName, platformURL, tt.role)
			if err != nil {
				t.Fatalf("RenderOrgRoleAssigned returned error: %v", err)
			}

			wantSubject := "User Role Assignment as " + tt.wantSubjectSfx
			if subject != wantSubject {
				t.Errorf("subject = %q, want %q", subject, wantSubject)
			}

			for _, want := range []string{orgName, tt.wantRoleLabel, platformURL} {
				if !strings.Contains(htmlBody, want) {
					t.Errorf("HTML body missing %q", want)
				}
			}

			for _, want := range []string{orgName, tt.wantRoleLabel, platformURL} {
				if !strings.Contains(textBody, want) {
					t.Errorf("text body missing %q", want)
				}
			}
			if textBody == "" {
				t.Error("text body must not be empty")
			}
		})
	}
}

func TestRenderOrgRoleAssigned_UnknownRole(t *testing.T) {
	t.Parallel()

	_, _, _, err := infraemail.RenderOrgRoleAssigned("Org", "https://example.com", "superadmin")
	if err == nil {
		t.Fatal("expected error for unsupported role, got nil")
	}
}
