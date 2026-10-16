// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

func TestNewProjectsAPI(t *testing.T) {
	svc := &service.ProjectsService{}
	authenticator := &auth.MockJWTAuth{}

	api := NewProjectsAPI(svc, authenticator)

	require.NotNil(t, api)
	assert.Same(t, svc, api.service)
	assert.Same(t, authenticator, api.auth)
}
