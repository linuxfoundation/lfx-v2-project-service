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
	const validProjectBody = `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":""}`

	tests := []struct {
		name               string
		body               string
		contentType        string
		wantStatus         int
		wantEndpointCalled bool
	}{
		{
			name:               "single parent uid key is accepted",
			body:               validProjectBody,
			contentType:        "application/json",
			wantStatus:         http.StatusCreated,
			wantEndpointCalled: true,
		},
		{
			name:               "three case-insensitive parent uid keys are rejected",
			body:               `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":"", "PARENT_UID":"", "Parent_Uid":""}`,
			contentType:        "application/json",
			wantStatus:         http.StatusBadRequest,
			wantEndpointCalled: false,
		},
		{
			name:               "nested parent uid keys are ignored",
			body:               `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":"", "metadata":{"parent_uid":"nested", "PARENT_UID":"nested"}}`,
			contentType:        "application/json",
			wantStatus:         http.StatusCreated,
			wantEndpointCalled: true,
		},
		{
			name:               "duplicate parent uid keys are rejected without content type",
			body:               `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":"", "PARENT_UID":""}`,
			wantStatus:         http.StatusBadRequest,
			wantEndpointCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
			assert.Equal(t, tt.wantEndpointCalled, endpointCalled)
		})
	}
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
