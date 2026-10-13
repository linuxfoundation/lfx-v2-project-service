// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"mime/multipart"
	"testing"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
}
