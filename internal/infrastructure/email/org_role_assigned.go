// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package email provides pre-rendered email templates used by NATS adapters
// to send transactional emails via the email service. It has no knowledge of
// NATS, ports, or email-service contracts — callers own the transport.
package email

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
)

//go:embed templates/org_role_assigned.html
var orgRoleAssignedHTML string

//go:embed templates/org_role_assigned.txt
var orgRoleAssignedTXT string

var orgRoleAssignedHTMLTmpl = template.Must(
	template.New("org_role_assigned.html").Parse(orgRoleAssignedHTML),
)

var orgRoleAssignedTXTTmpl = texttemplate.Must(
	texttemplate.New("org_role_assigned.txt").Parse(orgRoleAssignedTXT),
)

type orgRoleAssignedData struct {
	OrgName       string
	PlatformURL   string
	RoleLabel     string
	SubjectSuffix string
}

// RenderOrgRoleAssigned renders the v5 role-assignment email for a user who
// already has an LFID. Returns the email subject, HTML body, and plain-text body.
// role must be model.B2BOrgRoleWriter or model.B2BOrgRoleAuditor.
func RenderOrgRoleAssigned(orgName, platformURL, role string) (subject, htmlBody, textBody string, err error) {
	roleLabel, subjectSuffix, ok := orgRoleLabel(role)
	if !ok {
		return "", "", "", fmt.Errorf("unsupported role %q for role-assignment email", role)
	}

	td := orgRoleAssignedData{
		OrgName:       orgName,
		PlatformURL:   platformURL,
		RoleLabel:     roleLabel,
		SubjectSuffix: subjectSuffix,
	}

	var htmlBuf bytes.Buffer
	if err = orgRoleAssignedHTMLTmpl.Execute(&htmlBuf, td); err != nil {
		return "", "", "", fmt.Errorf("execute html template: %w", err)
	}

	var txtBuf bytes.Buffer
	if err = orgRoleAssignedTXTTmpl.Execute(&txtBuf, td); err != nil {
		return "", "", "", fmt.Errorf("execute text template: %w", err)
	}

	return "User Role Assignment as " + subjectSuffix, htmlBuf.String(), txtBuf.String(), nil
}

// orgRoleLabel maps a model role constant to the email display label (used
// twice in the body) and the subject suffix, following legacy ACS conventions.
func orgRoleLabel(role string) (label, subjectSuffix string, ok bool) {
	switch role {
	case model.B2BOrgRoleWriter:
		return "an administrator", "Company Administrator", true
	case model.B2BOrgRoleAuditor:
		return "a viewer", "viewer", true
	default:
		return "", "", false
	}
}
