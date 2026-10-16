// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

func TestNewProjectsAPI(t *testing.T) {
	svc := &service.ProjectsService{}

	api := NewProjectsAPI(svc)

	require.NotNil(t, api)
	assert.Same(t, svc, api.service)
}
