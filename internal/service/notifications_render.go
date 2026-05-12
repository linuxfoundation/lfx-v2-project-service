// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

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

type projectRoleNotificationData struct {
	RecipientName string
	ProjectName   string
	Role          string
	ProjectURL    string
	InviterName   string
}

// renderProjectRoleNotification renders HTML and plain-text bodies for a project role notification email.
func renderProjectRoleNotification(data projectRoleNotificationData) (subject, html, text string, err error) {
	subject = data.InviterName + " added you as a " + data.Role + " on " + data.ProjectName

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
