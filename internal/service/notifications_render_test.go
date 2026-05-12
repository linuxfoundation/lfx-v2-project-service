// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderProjectRoleNotification(t *testing.T) {
	data := projectRoleNotificationData{
		RecipientName: "Alice",
		ProjectName:   "Demo Project",
		Role:          "Writer",
		ProjectURL:    "https://dev.app.lfx.dev/projects/demo-project",
		InviterName:   "Bob",
	}

	subject, html, text, err := renderProjectRoleNotification(data)
	require.NoError(t, err)

	assert.Contains(t, subject, "Writer")
	assert.Contains(t, subject, "Demo Project")
	assert.Contains(t, subject, "Bob")

	assert.Contains(t, html, "Alice")
	assert.Contains(t, html, "Demo Project")
	assert.Contains(t, html, "Writer")
	assert.Contains(t, html, "https://dev.app.lfx.dev/projects/demo-project")
	assert.Contains(t, html, "Bob")
	assert.True(t, strings.Contains(html, "<html"), "expected HTML output")

	assert.Contains(t, text, "Alice")
	assert.Contains(t, text, "Demo Project")
	assert.Contains(t, text, "Writer")
	assert.Contains(t, text, "https://dev.app.lfx.dev/projects/demo-project")
	assert.Contains(t, text, "Bob")
	assert.False(t, strings.Contains(text, "<html"), "expected plain text output")
}
