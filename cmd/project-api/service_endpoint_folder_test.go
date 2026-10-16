// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

const (
	testProjectUID = "00000000-0000-0000-0000-000000000001"
	testFolderUID  = "00000000-0000-0000-0000-0000000000f1"
)

func TestCreateProjectFolder(t *testing.T) {
	tests := []struct {
		name       string
		payload    *projsvc.CreateProjectFolderPayload
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name:    "success",
			payload: &projsvc.CreateProjectFolderPayload{UID: testProjectUID, Name: "Governance"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.folder.On("UniqueFolderName", mock.Anything, mock.Anything).Return("lookup/key", nil)
				m.folder.On("CreateFolder", mock.Anything, mock.MatchedBy(func(f *models.ProjectFolder) bool {
					return f.ProjectUID == testProjectUID && f.Name == "Governance"
				})).Return(nil)
			},
		},
		{
			name:       "empty name is a bad request",
			payload:    &projsvc.CreateProjectFolderPayload{UID: testProjectUID, Name: ""},
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.CreateProjectFolderPayload{UID: testProjectUID, Name: "Governance"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(false, nil)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "duplicate folder name is a conflict",
			payload: &projsvc.CreateProjectFolderPayload{UID: testProjectUID, Name: "Governance"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.folder.On("UniqueFolderName", mock.Anything, mock.Anything).Return("", domain.ErrFolderNameExists)
			},
			wantErr: &projsvc.ConflictError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.CreateProjectFolder(context.Background(), tt.payload)

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.NotEmpty(t, *got.UID)
			assert.Equal(t, testProjectUID, *got.ProjectUID)
			assert.Equal(t, "Governance", *got.Name)
			m.folder.AssertExpectations(t)
		})
	}

	t.Run("service unavailable", func(t *testing.T) {
		api, _ := setupResourceAPI()
		api.service.FolderRepository = nil

		_, err := api.CreateProjectFolder(context.Background(), &projsvc.CreateProjectFolderPayload{UID: testProjectUID, Name: "Governance"})
		assert.IsType(t, &projsvc.ServiceUnavailableError{}, err)
	})
}

func TestGetProjectFolder(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name       string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name: "success returns folder and etag",
			setupMocks: func(m *resourceMocks) {
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(&models.ProjectFolder{
					UID: testFolderUID, ProjectUID: testProjectUID, Name: "Governance", CreatedAt: created, UpdatedAt: created,
				}, uint64(7), nil)
			},
		},
		{
			name: "not found",
			setupMocks: func(m *resourceMocks) {
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name: "repository failure is an internal error",
			setupMocks: func(m *resourceMocks) {
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.GetProjectFolder(context.Background(), &projsvc.GetProjectFolderPayload{UID: testProjectUID, FolderUID: testFolderUID})

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, testFolderUID, *got.Folder.UID)
			assert.Equal(t, "Governance", *got.Folder.Name)
			assert.Equal(t, "7", *got.Etag)
		})
	}
}

func TestDeleteProjectFolder(t *testing.T) {
	otherFolderUID := "00000000-0000-0000-0000-0000000000f2"

	tests := []struct {
		name       string
		ifMatch    *string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name:    "success deletes at the If-Match revision",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("ListLinks", mock.Anything, testProjectUID).Return([]*models.ProjectLink{{UID: "l1", FolderUID: &otherFolderUID}}, nil)
				m.document.On("ListDocuments", mock.Anything, testProjectUID).Return([]*models.ProjectDocument{}, nil)
				m.folder.On("DeleteFolder", mock.Anything, testProjectUID, testFolderUID, uint64(3)).Return(nil)
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
			name:    "folder containing a link is a conflict",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("ListLinks", mock.Anything, testProjectUID).Return([]*models.ProjectLink{{UID: "l1", FolderUID: misc.StringPtr(testFolderUID)}}, nil)
			},
			wantErr: &projsvc.ConflictError{},
		},
		{
			name:    "folder containing a document is a conflict",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("ListLinks", mock.Anything, testProjectUID).Return([]*models.ProjectLink{}, nil)
				m.document.On("ListDocuments", mock.Anything, testProjectUID).Return([]*models.ProjectDocument{{UID: "d1", FolderUID: misc.StringPtr(testFolderUID)}}, nil)
			},
			wantErr: &projsvc.ConflictError{},
		},
		{
			name:    "revision mismatch is a conflict",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("ListLinks", mock.Anything, testProjectUID).Return([]*models.ProjectLink{}, nil)
				m.document.On("ListDocuments", mock.Anything, testProjectUID).Return([]*models.ProjectDocument{}, nil)
				m.folder.On("DeleteFolder", mock.Anything, testProjectUID, testFolderUID, uint64(3)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: &projsvc.ConflictError{},
		},
		{
			name:    "folder not found",
			ifMatch: misc.StringPtr("3"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("ListLinks", mock.Anything, testProjectUID).Return([]*models.ProjectLink{}, nil)
				m.document.On("ListDocuments", mock.Anything, testProjectUID).Return([]*models.ProjectDocument{}, nil)
				m.folder.On("DeleteFolder", mock.Anything, testProjectUID, testFolderUID, uint64(3)).Return(domain.ErrFolderNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			err := api.DeleteProjectFolder(context.Background(), &projsvc.DeleteProjectFolderPayload{
				UID: testProjectUID, FolderUID: testFolderUID, IfMatch: tt.ifMatch,
			})

			if tt.wantErr != nil {
				assert.IsType(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			m.folder.AssertExpectations(t)
		})
	}
}
