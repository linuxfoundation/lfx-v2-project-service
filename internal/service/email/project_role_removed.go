// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"bytes"
	"embed"
	htmltemplate "html/template"
	texttemplate "text/template"
)

//go:embed templates/project_role_removed.html templates/project_role_removed.txt
var roleRemovedTemplates embed.FS

var (
	projectRoleRemovedHTMLTemplate = htmltemplate.Must(
		htmltemplate.New("project_role_removed.html").
			ParseFS(roleRemovedTemplates, "templates/project_role_removed.html"),
	)
	projectRoleRemovedTextTemplate = texttemplate.Must(
		texttemplate.New("project_role_removed.txt").
			ParseFS(roleRemovedTemplates, "templates/project_role_removed.txt"),
	)
)

// ProjectRoleRemovedData holds the template variables for a project role removal notification email.
type ProjectRoleRemovedData struct {
	RecipientName  string
	ProjectName    string
	OldRoles       []string
	OldJoinedRoles string // pre-computed by RenderProjectRoleRemoved
	InviterName    string
}

// RenderProjectRoleRemoved renders the subject, HTML body, and plain-text body
// for a role-removal notification email (user was fully removed from the project).
func RenderProjectRoleRemoved(data ProjectRoleRemovedData) (subject, html, text string, err error) {
	data.OldJoinedRoles = joinRoles(data.OldRoles)

	subject = "You have been removed from " + data.ProjectName

	var htmlBuf bytes.Buffer
	if err = projectRoleRemovedHTMLTemplate.Execute(&htmlBuf, data); err != nil {
		return
	}
	html = htmlBuf.String()

	var textBuf bytes.Buffer
	if err = projectRoleRemovedTextTemplate.Execute(&textBuf, data); err != nil {
		return
	}
	text = textBuf.String()
	return
}
