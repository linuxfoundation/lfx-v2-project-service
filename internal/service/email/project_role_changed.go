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

//go:embed templates/project_role_changed.html templates/project_role_changed.txt
var roleChangedTemplates embed.FS

var (
	projectRoleChangedHTMLTemplate = htmltemplate.Must(
		htmltemplate.New("project_role_changed.html").
			ParseFS(roleChangedTemplates, "templates/project_role_changed.html"),
	)
	projectRoleChangedTextTemplate = texttemplate.Must(
		texttemplate.New("project_role_changed.txt").
			ParseFS(roleChangedTemplates, "templates/project_role_changed.txt"),
	)
)

// ProjectRoleChangedData holds the template variables for a project role change notification email.
type ProjectRoleChangedData struct {
	RecipientName       string
	ProjectName         string
	OldRoles            []string
	NewRoles            []string
	OldJoinedRoles      string // pre-computed by RenderProjectRoleChanged
	NewJoinedRoles      string // pre-computed by RenderProjectRoleChanged
	OldRoleWord         string // "role" or "roles"; set automatically
	NewRoleWord         string // "role" or "roles"; set automatically
	ProjectURL          string
	InviterName         string
	NewCapabilityGroups []RoleCapabilityGroup // pre-computed by RenderProjectRoleChanged; set automatically
}

// RenderProjectRoleChanged renders the subject, HTML body, and plain-text body
// for a role-change notification email (user's role set was modified but they remain on the project).
func RenderProjectRoleChanged(data ProjectRoleChangedData) (subject, html, text string, err error) {
	if len(data.OldRoles) == 0 || len(data.NewRoles) == 0 {
		err = errors.New("email: OldRoles and NewRoles must both be non-empty")
		return
	}

	data.OldJoinedRoles = joinRoles(data.OldRoles)
	data.NewJoinedRoles = joinRoles(data.NewRoles)
	data.NewCapabilityGroups = capabilityGroupsFor(data.NewRoles)

	if len(data.OldRoles) == 1 {
		data.OldRoleWord = "role"
	} else {
		data.OldRoleWord = "roles"
	}
	if len(data.NewRoles) == 1 {
		data.NewRoleWord = "role"
	} else {
		data.NewRoleWord = "roles"
	}

	if data.InviterName != "" {
		subject = data.InviterName + " updated your role on " + data.ProjectName
	} else {
		subject = "Your role on " + data.ProjectName + " has been updated"
	}

	var htmlBuf bytes.Buffer
	if err = projectRoleChangedHTMLTemplate.Execute(&htmlBuf, data); err != nil {
		return
	}
	html = htmlBuf.String()

	var textBuf bytes.Buffer
	if err = projectRoleChangedTextTemplate.Execute(&textBuf, data); err != nil {
		return
	}
	text = textBuf.String()
	return
}
