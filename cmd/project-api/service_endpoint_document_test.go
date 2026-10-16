// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testDocumentUID = "00000000-0000-0000-0000-0000000000d1"

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

func TestUploadProjectDocument(t *testing.T) {
	payloadWith := func(mutate func(*projsvc.UploadProjectDocumentPayload)) *projsvc.UploadProjectDocumentPayload {
		p := &projsvc.UploadProjectDocumentPayload{
			UID:         testProjectUID,
			Name:        "Charter",
			FileName:    "charter.txt",
			ContentType: "text/plain",
			File:        []byte("hello world"),
		}
		if mutate != nil {
			mutate(p)
		}
		return p
	}

	tests := []struct {
		name       string
		payload    *projsvc.UploadProjectDocumentPayload
		setupMocks func(*resourceMocks)
		wantErr    any
		check      func(*testing.T, *projsvc.ProjectDocument)
	}{
		{
			name:    "success",
			payload: payloadWith(func(p *projsvc.UploadProjectDocumentPayload) { p.Description = misc.StringPtr("The charter") }),
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.document.On("UniqueDocumentName", mock.Anything, mock.Anything).Return("lookup/key", nil)
				m.document.On("PutDocumentFile", mock.Anything, mock.AnythingOfType("string"), []byte("hello world")).Return(nil)
				m.document.On("CreateDocumentMetadata", mock.Anything, mock.MatchedBy(func(d *models.ProjectDocument) bool {
					return d.ProjectUID == testProjectUID && d.Name == "Charter" && d.FileName == "charter.txt" &&
						d.Description == "The charter" && d.FileSize == 11
				})).Return(nil)
			},
			check: func(t *testing.T, got *projsvc.ProjectDocument) {
				assert.Equal(t, testProjectUID, *got.ProjectUID)
				assert.Equal(t, "Charter", *got.Name)
				assert.Equal(t, "charter.txt", *got.FileName)
				assert.Equal(t, int64(11), *got.FileSize)
				assert.Equal(t, "text/plain", *got.ContentType)
				assert.Equal(t, "The charter", *got.Description)
			},
		},
		{
			name:       "missing name is a bad request",
			payload:    payloadWith(func(p *projsvc.UploadProjectDocumentPayload) { p.Name = "" }),
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "empty file is a bad request",
			payload:    payloadWith(func(p *projsvc.UploadProjectDocumentPayload) { p.File = nil }),
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "disallowed content type is a bad request",
			payload:    payloadWith(func(p *projsvc.UploadProjectDocumentPayload) { p.ContentType = "application/x-msdownload" }),
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name: "oversized file is a bad request",
			payload: payloadWith(func(p *projsvc.UploadProjectDocumentPayload) {
				p.File = bytes.Repeat([]byte("a"), models.MaxDocumentFileSize+1)
			}),
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: payloadWith(nil),
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(false, nil)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "folder not found",
			payload: payloadWith(func(p *projsvc.UploadProjectDocumentPayload) { p.FolderUID = misc.StringPtr(testFolderUID) }),
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "duplicate document name is a conflict",
			payload: payloadWith(nil),
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.document.On("UniqueDocumentName", mock.Anything, mock.Anything).Return("", domain.ErrDocumentNameExists)
			},
			wantErr: &projsvc.ConflictError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.UploadProjectDocument(context.Background(), tt.payload)

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			tt.check(t, got)
			m.document.AssertExpectations(t)
		})
	}
}

func TestGetProjectDocument(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name       string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name: "success returns metadata and etag",
			setupMocks: func(m *resourceMocks) {
				m.document.On("GetDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID).Return(&models.ProjectDocument{
					UID: testDocumentUID, ProjectUID: testProjectUID, Name: "Charter", FileName: "charter.txt",
					ContentType: "text/plain", FileSize: 11, CreatedAt: created, UpdatedAt: created,
				}, uint64(5), nil)
			},
		},
		{
			name: "not found",
			setupMocks: func(m *resourceMocks) {
				m.document.On("GetDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID).Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name: "repository failure is an internal error",
			setupMocks: func(m *resourceMocks) {
				m.document.On("GetDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID).Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.GetProjectDocument(context.Background(), &projsvc.GetProjectDocumentPayload{UID: testProjectUID, DocumentUID: testDocumentUID})

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, testDocumentUID, *got.Document.UID)
			assert.Equal(t, "Charter", *got.Document.Name)
			assert.Equal(t, "5", *got.Etag)
		})
	}
}

func TestDownloadProjectDocument(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name: "success streams the file content",
			setupMocks: func(m *resourceMocks) {
				m.document.On("GetDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID).Return(&models.ProjectDocument{
					UID: testDocumentUID, FileName: "charter.txt", ContentType: "text/plain",
				}, uint64(5), nil)
				m.document.On("GetDocumentFile", mock.Anything, testDocumentUID).Return([]byte("hello world"), nil)
			},
		},
		{
			name: "document not found",
			setupMocks: func(m *resourceMocks) {
				m.document.On("GetDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID).Return(nil, uint64(0), domain.ErrDocumentNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			body, err := api.DownloadProjectDocument(context.Background(), &projsvc.DownloadProjectDocumentPayload{UID: testProjectUID, DocumentUID: testDocumentUID})

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, body)
				return
			}
			require.NoError(t, err)
			data, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, "hello world", string(data))
			assert.NoError(t, body.Close())

			// The returned body carries the metadata used for response headers.
			dl, ok := body.(*documentDownloadBody)
			require.True(t, ok)
			assert.Equal(t, "text/plain", dl.contentType)
			assert.Equal(t, "charter.txt", dl.fileName)
		})
	}
}

func TestDeleteProjectDocument(t *testing.T) {
	tests := []struct {
		name       string
		ifMatch    *string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name:    "success deletes at the If-Match revision",
			ifMatch: misc.StringPtr("6"),
			setupMocks: func(m *resourceMocks) {
				m.document.On("DeleteDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID, uint64(6)).Return(nil)
				m.document.On("DeleteDocumentFile", mock.Anything, testDocumentUID).Return(nil).Maybe()
			},
		},
		{
			name:       "missing If-Match is a bad request",
			ifMatch:    nil,
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "non-numeric If-Match is a bad request",
			ifMatch:    misc.StringPtr("abc"),
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "revision mismatch is a conflict",
			ifMatch: misc.StringPtr("6"),
			setupMocks: func(m *resourceMocks) {
				m.document.On("DeleteDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID, uint64(6)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: &projsvc.ConflictError{},
		},
		{
			name:    "document not found",
			ifMatch: misc.StringPtr("6"),
			setupMocks: func(m *resourceMocks) {
				m.document.On("DeleteDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID, uint64(6)).Return(domain.ErrDocumentNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			err := api.DeleteProjectDocument(context.Background(), &projsvc.DeleteProjectDocumentPayload{
				UID: testProjectUID, DocumentUID: testDocumentUID, IfMatch: tt.ifMatch,
			})

			if tt.wantErr != nil {
				assert.IsType(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			m.document.AssertCalled(t, "DeleteDocumentMetadata", mock.Anything, testProjectUID, testDocumentUID, uint64(6))
		})
	}
}

func TestDocumentDownloadBody(t *testing.T) {
	t.Run("Read drains the data then reports EOF", func(t *testing.T) {
		b := &documentDownloadBody{data: []byte("abcdef")}
		buf := make([]byte, 4)

		n, err := b.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "abcd", string(buf[:n]))

		n, err = b.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "ef", string(buf[:n]))

		_, err = b.Read(buf)
		assert.Equal(t, io.EOF, err)
	})

	tests := []struct {
		name                   string
		contentType            string
		fileName               string
		wantContentType        string
		wantContentDisposition string
	}{
		{
			name:                   "ascii file name",
			contentType:            "text/plain",
			fileName:               "charter.txt",
			wantContentType:        "text/plain",
			wantContentDisposition: `attachment; filename="charter.txt"; filename*=UTF-8''charter.txt`,
		},
		{
			name:                   "non-ascii file name gets an ascii fallback and an encoded form",
			contentType:            "application/pdf",
			fileName:               "résumé.pdf",
			wantContentType:        "application/pdf",
			wantContentDisposition: `attachment; filename="r_sum_.pdf"; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf`,
		},
		{
			name:                   "quotes and backslashes cannot break out of the quoted string",
			contentType:            "text/plain",
			fileName:               `a"b\c.txt`,
			wantContentType:        "text/plain",
			wantContentDisposition: `attachment; filename="a_b_c.txt"; filename*=UTF-8''a%22b%5Cc.txt`,
		},
		{
			name:                   "line breaks cannot inject headers",
			contentType:            "text/plain",
			fileName:               "a\r\nX-Injected: 1.txt",
			wantContentType:        "text/plain",
			wantContentDisposition: `attachment; filename="a__X-Injected: 1.txt"; filename*=UTF-8''a%0D%0AX-Injected:%201.txt`,
		},
		{
			// With no metadata the body sets no headers; the recorder then sniffs a
			// Content-Type on first write, and no Content-Disposition is set.
			name:            "empty metadata sets no headers",
			wantContentType: "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run("WriteTo "+tt.name, func(t *testing.T) {
			b := &documentDownloadBody{data: []byte("hello"), contentType: tt.contentType, fileName: tt.fileName}
			rec := httptest.NewRecorder()

			n, err := b.WriteTo(rec)

			require.NoError(t, err)
			assert.Equal(t, int64(5), n)
			assert.Equal(t, "hello", rec.Body.String())
			assert.Equal(t, tt.wantContentType, rec.Header().Get("Content-Type"))
			assert.Equal(t, tt.wantContentDisposition, rec.Header().Get("Content-Disposition"))
		})
	}

	t.Run("WriteTo writes the body to a plain writer without headers", func(t *testing.T) {
		b := &documentDownloadBody{data: []byte("hello"), contentType: "text/plain", fileName: "charter.txt"}
		var buf bytes.Buffer

		n, err := b.WriteTo(&buf)

		require.NoError(t, err)
		assert.Equal(t, int64(5), n)
		assert.Equal(t, "hello", buf.String())
	})
}

func TestContentDispositionHeader(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		want     string
	}{
		{name: "ascii file name", fileName: "charter.txt", want: `attachment; filename="charter.txt"; filename*=UTF-8''charter.txt`},
		{name: "spaces are escaped in the encoded form only", fileName: "annual report.pdf", want: `attachment; filename="annual report.pdf"; filename*=UTF-8''annual%20report.pdf`},
		{name: "non-ascii characters are replaced in the fallback", fileName: "résumé.pdf", want: `attachment; filename="r_sum_.pdf"; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf`},
		{name: "quotes and backslashes cannot break out of the quoted string", fileName: `a"b\c.txt`, want: `attachment; filename="a_b_c.txt"; filename*=UTF-8''a%22b%5Cc.txt`},
		{name: "line breaks cannot inject headers", fileName: "a\r\nX-Injected: 1.txt", want: `attachment; filename="a__X-Injected: 1.txt"; filename*=UTF-8''a%0D%0AX-Injected:%201.txt`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, contentDispositionHeader(tt.fileName))
		})
	}
}
