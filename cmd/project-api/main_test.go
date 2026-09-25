// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	projsvchttpserver "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"
)

func TestCreateProjectRejectsDuplicateCaseInsensitiveParentUID(t *testing.T) {
	endpointCalled := false
	handler := projsvchttpserver.NewCreateProjectHandler(
		func(context.Context, any) (any, error) {
			endpointCalled = true
			return &projsvc.ProjectFull{}, nil
		},
		goahttp.NewMuxer(),
		projectRequestDecoder,
		goahttp.ResponseEncoder,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":"", "PARENT_UID":""}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
	assert.False(t, endpointCalled)
}

func TestLFXSelfServeBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		environment string
		want        string
	}{
		{
			name:    "explicit base URL takes precedence",
			baseURL: "https://custom.example.com",
			want:    "https://custom.example.com",
		},
		{
			name:        "prod environment",
			environment: "prod",
			want:        "https://app.lfx.dev",
		},
		{
			name:        "production alias",
			environment: "production",
			want:        "https://app.lfx.dev",
		},
		{
			name:        "staging environment",
			environment: "staging",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "stg alias",
			environment: "stg",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "stage alias",
			environment: "stage",
			want:        "https://app.staging.lfx.dev",
		},
		{
			name:        "dev environment",
			environment: "dev",
			want:        "https://app.dev.lfx.dev",
		},
		{
			name:        "development alias",
			environment: "development",
			want:        "https://app.dev.lfx.dev",
		},
		{
			name: "unset environment defaults to prod",
			want: "https://app.lfx.dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LFX_SELF_SERVE_BASE_URL", tt.baseURL)
			t.Setenv("LFX_ENVIRONMENT", tt.environment)

			assert.Equal(t, tt.want, LFXSelfServeBaseURL())
		})
	}
}
