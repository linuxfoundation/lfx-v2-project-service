// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name  string
		flags cmdFlags
		want  string
	}{
		{name: "wildcard bind listens on all interfaces", flags: cmdFlags{Bind: "*", Port: "8080"}, want: ":8080"},
		{name: "explicit interface", flags: cmdFlags{Bind: "127.0.0.1", Port: "9090"}, want: "127.0.0.1:9090"},
		{name: "hostname", flags: cmdFlags{Bind: "localhost", Port: "8080"}, want: "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, listenAddr(tt.flags))
		})
	}
}

func TestNewHTTPHandler(t *testing.T) {
	koDataDir := t.TempDir()
	// Goa serves /_projects/openapi.json from <KO_DATA_PATH>/gen/http/openapi.json,
	// which is what the kodata/gen symlink provides in the built image.
	require.NoError(t, os.MkdirAll(filepath.Join(koDataDir, "gen", "http"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(koDataDir, "gen", "http", "openapi.json"), []byte(`{"openapi":"test"}`), 0o600))
	t.Setenv("KO_DATA_PATH", koDataDir)

	t.Run("livez responds and sets a request id", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/livez", nil))

		assert.Equal(t, http.StatusOK, res.Code)
		assert.Equal(t, "OK\n", res.Body.String())
		assert.NotEmpty(t, res.Header().Get(constants.RequestIDHeader))
	})

	t.Run("an existing request id is echoed back", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		req := httptest.NewRequest(http.MethodGet, "/livez", nil)
		req.Header.Set(constants.RequestIDHeader, "request-123")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		assert.Equal(t, "request-123", res.Header().Get(constants.RequestIDHeader))
	})

	t.Run("readyz is ok when the service is ready", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		assert.Equal(t, http.StatusOK, res.Code)
	})

	t.Run("readyz is unavailable when the service is not ready", func(t *testing.T) {
		api, _, _ := setupAPI()
		api.service.ProjectRepository = nil
		handler := newHTTPHandler(api)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		assert.Equal(t, http.StatusServiceUnavailable, res.Code)
	})

	t.Run("unknown paths are not found", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))

		assert.Equal(t, http.StatusNotFound, res.Code)
	})

	t.Run("openapi spec is served from KO_DATA_PATH", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/_projects/openapi.json", nil))

		assert.Equal(t, http.StatusOK, res.Code)
		assert.JSONEq(t, `{"openapi":"test"}`, res.Body.String())
	})

	t.Run("etag header is set from the result on get endpoints", func(t *testing.T) {
		// Sequential fixture UUID with the v4 version and variant bits set: the
		// generated Goa validator rejects the all-zero testProjectUID in a path.
		const uid = "00000000-0000-4000-8000-000000000001"

		tests := []struct {
			name     string
			path     string
			wantEtag string
		}{
			{name: "project base", path: "/projects/" + uid + "?v=1", wantEtag: "4"},
			{name: "project settings", path: "/projects/" + uid + "/settings?v=1", wantEtag: "8"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api, mockRepo, _ := setupAPI()
				api.auth.(*auth.MockJWTAuth).On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("test-user", "", nil)
				mockRepo.On("GetProjectBaseWithRevision", mock.Anything, uid).Return(&models.ProjectBase{UID: uid, Slug: "test-project", Name: "Test Project"}, uint64(4), nil)
				mockRepo.On("ProjectExists", mock.Anything, uid).Return(true, nil)
				mockRepo.On("GetProjectSettingsWithRevision", mock.Anything, uid).Return(&models.ProjectSettings{UID: uid}, uint64(8), nil)
				handler := newHTTPHandler(api)

				req := httptest.NewRequest(http.MethodGet, tt.path, nil)
				req.Header.Set("Authorization", "Bearer test-token")
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				require.Equal(t, http.StatusOK, res.Code)
				assert.Equal(t, tt.wantEtag, res.Header().Get("ETag"))
			})
		}
	})

	t.Run("request bodies above the limit are rejected before the endpoint runs", func(t *testing.T) {
		api, _, _ := setupAPI()
		mockAuth := api.auth.(*auth.MockJWTAuth)
		// If the body were read in full, the request would reach the security
		// handler, which fails the request with this error.
		mockAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("", "", errors.New("auth reached"))
		handler := newHTTPHandler(api)

		// A well-formed create-project body padded past the limit with an
		// unknown field, so only the size limit can make the request fail.
		padding := strings.Repeat("a", int(models.MaxDocumentFileSize)+(2<<20))
		body := `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":"","padding":"` + padding + `"}`
		req := httptest.NewRequest(http.MethodPost, "/projects?v=1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		assert.Equal(t, http.StatusBadRequest, res.Code)
		mockAuth.AssertNotCalled(t, "ParsePrincipalAndEmail", mock.Anything, mock.Anything)
	})

	t.Run("request bodies within the limit reach the endpoint", func(t *testing.T) {
		api, _, _ := setupAPI()
		mockAuth := api.auth.(*auth.MockJWTAuth)
		mockAuth.On("ParsePrincipalAndEmail", mock.Anything, mock.Anything).Return("", "", errors.New("auth reached"))
		handler := newHTTPHandler(api)

		body := `{"slug":"test-project","name":"Test Project","description":"Test description","parent_uid":""}`
		req := httptest.NewRequest(http.MethodPost, "/projects?v=1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		mockAuth.AssertCalled(t, "ParsePrincipalAndEmail", mock.Anything, mock.Anything)
	})
}
