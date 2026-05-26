// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package email

import (
	"bytes"
	"embed"
	htmltemplate "html/template"
	texttemplate "text/template"
)

//go:embed templates/project_role_notification.html templates/project_role_notification.txt
var notificationTemplates embed.FS

var (
	projectRoleHTMLTemplate = htmltemplate.Must(
		htmltemplate.New("project_role_notification.html").
			ParseFS(notificationTemplates, "templates/project_role_notification.html"),
	)
	projectRoleTextTemplate = texttemplate.Must(
		texttemplate.New("project_role_notification.txt").
			ParseFS(notificationTemplates, "templates/project_role_notification.txt"),
	)
)

// ProjectRoleNotificationData holds the template variables for a project role notification email.
type ProjectRoleNotificationData struct {
	RecipientName string
	ProjectName   string
	Roles         []string
	JoinedRoles   string // pre-computed by RenderProjectRoleNotification; set automatically
	RoleWord      string // "role" (single) or "roles" (multiple); set automatically
	Article       string // "a " (single) or "" (multiple); set automatically
	ProjectURL    string
	InviterName   string
}

// RenderProjectRoleNotification renders the subject, HTML body, and plain-text body
// for a project role notification email (user added to a project).
func RenderProjectRoleNotification(data ProjectRoleNotificationData) (subject, html, text string, err error) {
	data.JoinedRoles = joinRoles(data.Roles)
	if len(data.Roles) == 1 {
		data.RoleWord = "role"
		data.Article = "a "
	} else {
		data.RoleWord = "roles"
		data.Article = ""
	}

	if data.InviterName != "" {
		subject = data.InviterName + " added you as " + data.Article + data.JoinedRoles + " " + data.RoleWord + " on " + data.ProjectName
	} else {
		subject = "You have been added as " + data.Article + data.JoinedRoles + " " + data.RoleWord + " on " + data.ProjectName
	}

	var htmlBuf bytes.Buffer
	if err = projectRoleHTMLTemplate.Execute(&htmlBuf, data); err != nil {
		return
	}
	html = htmlBuf.String()

	var textBuf bytes.Buffer
	if err = projectRoleTextTemplate.Execute(&textBuf, data); err != nil {
		return
	}
	text = textBuf.String()
	return
}
