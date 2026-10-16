// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	projsvchttpserver "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
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

// errReader is an io.Reader that always fails.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestProjectRequestDecoder(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		wantCustom  bool
	}{
		{name: "POST /projects with JSON content type uses the custom decoder", method: http.MethodPost, path: "/projects", contentType: "application/json", wantCustom: true},
		{name: "JSON content type parameters are ignored", method: http.MethodPost, path: "/projects", contentType: "application/json; charset=utf-8", wantCustom: true},
		{name: "missing content type is treated as JSON", method: http.MethodPost, path: "/projects", wantCustom: true},
		{name: "non-JSON content type falls through to the default decoder", method: http.MethodPost, path: "/projects", contentType: "text/plain"},
		{name: "vendor JSON content type falls through to the default decoder", method: http.MethodPost, path: "/projects", contentType: "application/vnd.api+json"},
		{name: "malformed content type falls through to the default decoder", method: http.MethodPost, path: "/projects", contentType: "///"},
		{name: "other methods fall through to the default decoder", method: http.MethodPut, path: "/projects", contentType: "application/json"},
		{name: "other paths fall through to the default decoder", method: http.MethodPost, path: "/projects/abc", contentType: "application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := projectRequestDecoder(req)

			_, isCustom := got.(*projectJSONDecoder)
			assert.Equal(t, tt.wantCustom, isCustom)
		})
	}

	t.Run("valid body is decoded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"slug":"test-project"}`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		require.NoError(t, projectRequestDecoder(req).Decode(&dst))
		assert.Equal(t, "test-project", dst["slug"])
	})

	t.Run("duplicate parent_uid keys fail on decode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"parent_uid":"a","PARENT_UID":"b"}`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		err := projectRequestDecoder(req).Decode(&dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multiple parent_uid keys")
		assert.Nil(t, dst)
	})

	t.Run("malformed JSON fails on decode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"slug":`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		assert.Error(t, projectRequestDecoder(req).Decode(&dst))
	})

	t.Run("body read failure is surfaced on decode", func(t *testing.T) {
		readErr := errors.New("connection reset")
		req := httptest.NewRequest(http.MethodPost, "/projects", errReader{err: readErr})
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		assert.ErrorIs(t, projectRequestDecoder(req).Decode(&dst), readErr)
	})
}

func TestRejectDuplicateParentUIDKeys(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "empty object", body: `{}`},
		{name: "no parent_uid", body: `{"slug":"a","name":"b"}`},
		{name: "single parent_uid", body: `{"parent_uid":"a"}`},
		{name: "values containing braces and arrays are skipped", body: `{"a":{"b":[1,2,{"c":3}]},"parent_uid":"a"}`},
		{name: "nested duplicates are ignored", body: `{"parent_uid":"a","meta":{"parent_uid":"x","PARENT_UID":"y"}}`},
		{name: "exact duplicate", body: `{"parent_uid":"a","parent_uid":"b"}`, wantErr: "multiple parent_uid keys"},
		{name: "case-insensitive duplicate", body: `{"parent_uid":"a","Parent_UID":"b"}`, wantErr: "multiple parent_uid keys"},
		{name: "array body", body: `[]`, wantErr: "must be a JSON object"},
		{name: "string body", body: `"parent_uid"`, wantErr: "must be a JSON object"},
		{name: "empty body", body: ``, wantErr: "EOF"},
		{name: "truncated before value", body: `{"parent_uid":`, wantErr: "EOF"},
		{name: "truncated before closing brace", body: `{"parent_uid":"a"`, wantErr: "EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectDuplicateParentUIDKeys(json.NewDecoder(strings.NewReader(tt.body)))

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

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

	t.Run("request bodies above the limit are rejected", func(t *testing.T) {
		api, _, _ := setupAPI()
		handler := newHTTPHandler(api)

		// Larger than the document size limit plus the multipart headroom.
		body := strings.NewReader(strings.Repeat("a", 12<<20))
		req := httptest.NewRequest(http.MethodPost, "/projects", body)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		assert.GreaterOrEqual(t, res.Code, http.StatusBadRequest)
		assert.NotEqual(t, http.StatusCreated, res.Code)
	})
}
