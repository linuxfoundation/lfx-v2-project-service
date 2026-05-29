// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"bytes"
	"embed"
	"errors"
	htmltemplate "html/template"
	texttemplate "text/template"
)

//go:embed templates/project_document_uploaded.html templates/project_document_uploaded.txt
var documentUploadedTemplates embed.FS

var (
	documentUploadedHTMLTemplate = htmltemplate.Must(
		htmltemplate.New("project_document_uploaded.html").
			ParseFS(documentUploadedTemplates, "templates/project_document_uploaded.html"),
	)
	documentUploadedTextTemplate = texttemplate.Must(
		texttemplate.New("project_document_uploaded.txt").
			ParseFS(documentUploadedTemplates, "templates/project_document_uploaded.txt"),
	)
)

// ProjectDocumentUploadedData holds the template variables for a document/link upload notification email.
type ProjectDocumentUploadedData struct {
	RecipientName string
	ProjectName   string
	DocumentName  string
	DocumentType  string // "file" | "link"
	FileName      string // set for files
	URL           string // set for links
	UploaderName  string
	ProjectURL    string
}

// RenderProjectDocumentUploaded renders the subject, HTML body, and plain-text body for a
// document/link upload notification email sent to project writers and auditors.
func RenderProjectDocumentUploaded(data ProjectDocumentUploadedData) (subject, html, text string, err error) {
	if data.DocumentName == "" {
		err = errors.New("email: DocumentName must be non-empty")
		return
	}
	if data.ProjectName == "" {
		err = errors.New("email: ProjectName must be non-empty")
		return
	}

	docType := "document"
	if data.DocumentType == "link" {
		docType = "link"
	}
	if data.UploaderName != "" {
		subject = data.UploaderName + " added " + data.DocumentName + " " + docType + " to " + data.ProjectName
	} else {
		subject = "A new " + docType + " was added to " + data.ProjectName
	}

	var htmlBuf bytes.Buffer
	if err = documentUploadedHTMLTemplate.Execute(&htmlBuf, data); err != nil {
		return
	}
	html = htmlBuf.String()

	var textBuf bytes.Buffer
	if err = documentUploadedTextTemplate.Execute(&textBuf, data); err != nil {
		return
	}
	text = textBuf.String()
	return
}
