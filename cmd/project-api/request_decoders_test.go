// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	projsvchttpserver "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"
)

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
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(`{}`))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			got := projectRequestDecoder(req)

			_, isCustom := got.(*projectJSONDecoder)
			assert.Equal(t, tt.wantCustom, isCustom)
		})
	}

	t.Run("valid body is decoded", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects", strings.NewReader(`{"slug":"test-project"}`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		require.NoError(t, projectRequestDecoder(req).Decode(&dst))
		assert.Equal(t, "test-project", dst["slug"])
	})

	t.Run("duplicate parent_uid keys fail on decode", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects", strings.NewReader(`{"parent_uid":"a","PARENT_UID":"b"}`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		err := projectRequestDecoder(req).Decode(&dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multiple parent_uid keys")
		assert.Nil(t, dst)
	})

	t.Run("malformed JSON fails on decode", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects", strings.NewReader(`{"slug":`))
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		assert.Error(t, projectRequestDecoder(req).Decode(&dst))
	})

	t.Run("body read failure is surfaced on decode", func(t *testing.T) {
		readErr := errors.New("connection reset")
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects", errReader{err: readErr})
		req.Header.Set("Content-Type", "application/json")

		var dst map[string]any
		assert.ErrorIs(t, projectRequestDecoder(req).Decode(&dst), readErr)
	})

	t.Run("create-project handler validates the request body", func(t *testing.T) {
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
			{
				name:               "slug starting with a digit is accepted",
				body:               `{"slug":"3mf-project","name":"Test Project","description":"Test description","parent_uid":""}`,
				contentType:        "application/json",
				wantStatus:         http.StatusCreated,
				wantEndpointCalled: true,
			},
			{
				name:               "slug starting with a hyphen is rejected",
				body:               `{"slug":"-test-project","name":"Test Project","description":"Test description","parent_uid":""}`,
				contentType:        "application/json",
				wantStatus:         http.StatusBadRequest,
				wantEndpointCalled: false,
			},
			{
				name:               "slug with uppercase letters is rejected",
				body:               `{"slug":"Test-project","name":"Test Project","description":"Test description","parent_uid":""}`,
				contentType:        "application/json",
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

				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/projects", strings.NewReader(tt.body))
				if tt.contentType != "" {
					req.Header.Set("Content-Type", tt.contentType)
				}
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)

				require.Equal(t, tt.wantStatus, res.Code)
				assert.Equal(t, tt.wantEndpointCalled, endpointCalled)
			})
		}
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

func TestUploadDocumentDecoder(t *testing.T) {
	const validFolderUID = "00000000-0000-0000-0000-0000000000f1"

	tests := []struct {
		name          string
		folderUID     string
		wantErr       bool
		wantFolderUID *string
	}{
		{name: "valid folder uid accepted", folderUID: validFolderUID, wantFolderUID: misc.StringPtr(validFolderUID)},
		{name: "empty folder uid treated as absent", folderUID: ""},
		{name: "reservation key rejected", folderUID: "lookup/project-folders/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "non-uuid rejected", folderUID: "folder-1", wantErr: true},
		{name: "urn form rejected", folderUID: "urn:uuid:" + validFolderUID, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			require.NoError(t, w.WriteField("folder_uid", tt.folderUID))
			require.NoError(t, w.Close())

			var payload *projsvc.UploadProjectDocumentPayload
			err := uploadDocumentDecoder(multipart.NewReader(&buf, w.Boundary()), &payload)

			if tt.wantErr {
				var badReq *projsvc.BadRequestError
				require.ErrorAs(t, err, &badReq)
				assert.Equal(t, "400", badReq.Code)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFolderUID, payload.FolderUID)
		})
	}

	// decode runs the decoder over a multipart body built by build.
	decode := func(t *testing.T, build func(w *multipart.Writer)) (*projsvc.UploadProjectDocumentPayload, error) {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		build(w)
		require.NoError(t, w.Close())

		var payload *projsvc.UploadProjectDocumentPayload
		err := uploadDocumentDecoder(multipart.NewReader(&buf, w.Boundary()), &payload)
		return payload, err
	}

	// filePart writes a file part with an explicit part Content-Type.
	filePart := func(t *testing.T, w *multipart.Writer, fileName, contentType string, data []byte) {
		t.Helper()
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, fileName))
		h.Set("Content-Type", contentType)
		part, err := w.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}

	t.Run("text fields and file are decoded", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("name", "Charter"))
			require.NoError(t, w.WriteField("description", "The charter"))
			require.NoError(t, w.WriteField("file_name", "charter.txt"))
			require.NoError(t, w.WriteField("content_type", "text/plain"))
			filePart(t, w, "ignored.bin", "application/octet-stream", []byte("hello"))
		})
		require.NoError(t, err)
		assert.Equal(t, "Charter", payload.Name)
		assert.Equal(t, misc.StringPtr("The charter"), payload.Description)
		assert.Equal(t, "charter.txt", payload.FileName)
		assert.Equal(t, "text/plain", payload.ContentType)
		assert.Equal(t, []byte("hello"), payload.File)
	})

	t.Run("content_type media type parameters are stripped", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("content_type", "text/plain; charset=utf-8"))
		})
		require.NoError(t, err)
		assert.Equal(t, "text/plain", payload.ContentType)
	})

	t.Run("unparseable content_type is kept as sent", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("content_type", "not a media type"))
		})
		require.NoError(t, err)
		assert.Equal(t, "not a media type", payload.ContentType)
	})

	t.Run("file part headers fill in file name and content type when fields are absent", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			filePart(t, w, "notes.txt", "text/plain; charset=utf-8", []byte("hello"))
		})
		require.NoError(t, err)
		assert.Equal(t, "notes.txt", payload.FileName)
		assert.Equal(t, "text/plain", payload.ContentType)
	})

	t.Run("explicit fields take precedence over file part headers", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("file_name", "explicit.txt"))
			require.NoError(t, w.WriteField("content_type", "text/csv"))
			filePart(t, w, "from-part.txt", "text/plain", []byte("hello"))
		})
		require.NoError(t, err)
		assert.Equal(t, "explicit.txt", payload.FileName)
		assert.Equal(t, "text/csv", payload.ContentType)
	})

	t.Run("unknown parts are ignored", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("unexpected", "value"))
			require.NoError(t, w.WriteField("name", "Charter"))
		})
		require.NoError(t, err)
		assert.Equal(t, "Charter", payload.Name)
	})

	t.Run("existing payload is reused", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		require.NoError(t, w.WriteField("name", "Charter"))
		require.NoError(t, w.Close())

		payload := &projsvc.UploadProjectDocumentPayload{UID: "proj-1"}
		require.NoError(t, uploadDocumentDecoder(multipart.NewReader(&buf, w.Boundary()), &payload))
		assert.Equal(t, "proj-1", payload.UID)
		assert.Equal(t, "Charter", payload.Name)
	})

	for _, field := range []string{"name", "description", "folder_uid", "file_name", "content_type"} {
		t.Run("oversized "+field+" is rejected", func(t *testing.T) {
			_, err := decode(t, func(w *multipart.Writer) {
				require.NoError(t, w.WriteField(field, strings.Repeat("a", maxTextPartSize+1)))
			})
			var badReq *projsvc.BadRequestError
			require.ErrorAs(t, err, &badReq)
			assert.Equal(t, "400", badReq.Code)
		})
	}

	t.Run("text field at the size limit is accepted", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			require.NoError(t, w.WriteField("name", strings.Repeat("a", maxTextPartSize)))
		})
		require.NoError(t, err)
		assert.Len(t, payload.Name, maxTextPartSize)
	})

	t.Run("file is capped one byte past the maximum so oversize can be detected", func(t *testing.T) {
		payload, err := decode(t, func(w *multipart.Writer) {
			filePart(t, w, "big.txt", "text/plain", bytes.Repeat([]byte("a"), models.MaxDocumentFileSize+100))
		})
		require.NoError(t, err)
		assert.Len(t, payload.File, models.MaxDocumentFileSize+1)
	})
}
