// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProjectDocumentUploaded(t *testing.T) {
	tests := []struct {
		name        string
		data        ProjectDocumentUploadedData
		wantSubject []string
		wantHTML    []string
		wantText    []string
		wantErr     bool
	}{
		{
			name: "file upload with uploader",
			data: ProjectDocumentUploadedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				DocumentName:  "Q4 Report",
				DocumentType:  "file",
				FileName:      "q4_report.pdf",
				UploaderName:  "Bob",
				ProjectURL:    "https://app.dev.lfx.dev/project/overview?project=demo-project",
			},
			wantSubject: []string{"Bob", "Q4 Report", "document", "Demo Project"},
			wantHTML:    []string{"Alice", "Bob", "Q4 Report", "q4_report.pdf", "Demo Project", "https://app.dev.lfx.dev/project/overview?project=demo-project"},
			wantText:    []string{"Alice", "Bob", "Q4 Report", "q4_report.pdf", "Demo Project", "https://app.dev.lfx.dev/project/overview?project=demo-project"},
		},
		{
			name: "link upload with uploader",
			data: ProjectDocumentUploadedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				DocumentName:  "Governance Docs",
				DocumentType:  "link",
				URL:           "https://example.com/governance",
				UploaderName:  "Bob",
				ProjectURL:    "https://app.dev.lfx.dev/project/overview?project=demo-project",
			},
			wantSubject: []string{"Bob", "Governance Docs", "link", "Demo Project"},
			wantHTML:    []string{"Alice", "Bob", "Governance Docs", "https://example.com/governance", "Demo Project"},
			wantText:    []string{"Alice", "Bob", "Governance Docs", "https://example.com/governance"},
		},
		{
			name: "file upload without uploader — fallback subject",
			data: ProjectDocumentUploadedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				DocumentName:  "Charter",
				DocumentType:  "file",
				FileName:      "charter.pdf",
				ProjectURL:    "https://app.dev.lfx.dev/project/overview?project=demo-project",
			},
			wantSubject: []string{"A new document was added to", "Demo Project"},
			wantHTML:    []string{"Alice", "Charter", "charter.pdf", "Demo Project"},
			wantText:    []string{"Alice", "Charter", "charter.pdf"},
		},
		{
			name: "link upload without uploader — fallback subject says link",
			data: ProjectDocumentUploadedData{
				RecipientName: "Carol",
				ProjectName:   "Another Project",
				DocumentName:  "Spec Link",
				DocumentType:  "link",
				URL:           "https://specs.example.com",
				ProjectURL:    "https://app.dev.lfx.dev/project/overview?project=another",
			},
			wantSubject: []string{"A new link was added to", "Another Project"},
			wantHTML:    []string{"Carol", "Spec Link", "https://specs.example.com"},
			wantText:    []string{"Carol", "Spec Link", "https://specs.example.com"},
		},
		{
			name: "missing document name returns error",
			data: ProjectDocumentUploadedData{
				RecipientName: "Alice",
				ProjectName:   "Demo Project",
				DocumentType:  "file",
				ProjectURL:    "https://app.dev.lfx.dev",
			},
			wantErr: true,
		},
		{
			name: "missing project name returns error",
			data: ProjectDocumentUploadedData{
				RecipientName: "Alice",
				DocumentName:  "Some Doc",
				DocumentType:  "file",
				ProjectURL:    "https://app.dev.lfx.dev",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, html, text, err := RenderProjectDocumentUploaded(tt.data)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
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
