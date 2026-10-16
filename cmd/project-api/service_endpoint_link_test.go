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

const testLinkUID = "00000000-0000-0000-0000-0000000000a1"

func TestCreateProjectLink(t *testing.T) {
	tests := []struct {
		name            string
		payload         *projsvc.CreateProjectLinkPayload
		setupMocks      func(*resourceMocks)
		wantErr         any
		wantDescription *string
		wantFolderUID   *string
	}{
		{
			name:    "success without description or folder",
			payload: &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.link.On("CreateLink", mock.Anything, mock.MatchedBy(func(l *models.ProjectLink) bool {
					return l.ProjectUID == testProjectUID && l.Name == "Website" && l.URL == "https://example.com" &&
						l.Description == "" && l.FolderUID == nil
				})).Return(nil)
			},
		},
		{
			name: "success with description and folder",
			payload: &projsvc.CreateProjectLinkPayload{
				UID: testProjectUID, Name: "Website", URL: "https://example.com",
				Description: misc.StringPtr("Project site"), FolderUID: misc.StringPtr(testFolderUID),
			},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(&models.ProjectFolder{UID: testFolderUID}, uint64(1), nil)
				m.link.On("CreateLink", mock.Anything, mock.MatchedBy(func(l *models.ProjectLink) bool {
					return l.Description == "Project site" && l.FolderUID != nil && *l.FolderUID == testFolderUID
				})).Return(nil)
			},
			wantDescription: misc.StringPtr("Project site"),
			wantFolderUID:   misc.StringPtr(testFolderUID),
		},
		{
			name:       "empty name is a bad request",
			payload:    &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "", URL: "https://example.com"},
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "non-http url is a bad request",
			payload:    &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "ftp://example.com"},
			setupMocks: func(*resourceMocks) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "invalid folder uid is a bad request",
			payload: &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com", FolderUID: misc.StringPtr("folder-1")},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
			},
			wantErr: &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(false, nil)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "folder not found",
			payload: &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com", FolderUID: misc.StringPtr(testFolderUID)},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.folder.On("GetFolder", mock.Anything, testProjectUID, testFolderUID).Return(nil, uint64(0), domain.ErrFolderNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "repository failure is an internal error",
			payload: &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com"},
			setupMocks: func(m *resourceMocks) {
				m.project.On("ProjectExists", mock.Anything, testProjectUID).Return(true, nil)
				m.link.On("CreateLink", mock.Anything, mock.Anything).Return(domain.ErrInternal)
			},
			wantErr: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.CreateProjectLink(context.Background(), tt.payload)

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, testProjectUID, *got.ProjectUID)
			assert.Equal(t, tt.payload.Name, *got.Name)
			assert.Equal(t, tt.payload.URL, *got.URL)
			assert.Equal(t, tt.wantDescription, got.Description)
			assert.Equal(t, tt.wantFolderUID, got.FolderUID)
			m.link.AssertExpectations(t)
		})
	}

	t.Run("service unavailable", func(t *testing.T) {
		api, _ := setupResourceAPI()
		api.service.LinkRepository = nil

		_, err := api.CreateProjectLink(context.Background(), &projsvc.CreateProjectLinkPayload{UID: testProjectUID, Name: "Website", URL: "https://example.com"})
		assert.IsType(t, &projsvc.ServiceUnavailableError{}, err)
	})
}

func TestGetProjectLink(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name       string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name: "success returns link and etag",
			setupMocks: func(m *resourceMocks) {
				m.link.On("GetLink", mock.Anything, testProjectUID, testLinkUID).Return(&models.ProjectLink{
					UID: testLinkUID, ProjectUID: testProjectUID, Name: "Website", URL: "https://example.com", CreatedAt: created, UpdatedAt: created,
				}, uint64(9), nil)
			},
		},
		{
			name: "not found",
			setupMocks: func(m *resourceMocks) {
				m.link.On("GetLink", mock.Anything, testProjectUID, testLinkUID).Return(nil, uint64(0), domain.ErrLinkNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name: "repository failure is an internal error",
			setupMocks: func(m *resourceMocks) {
				m.link.On("GetLink", mock.Anything, testProjectUID, testLinkUID).Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			got, err := api.GetProjectLink(context.Background(), &projsvc.GetProjectLinkPayload{UID: testProjectUID, LinkUID: testLinkUID})

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, testLinkUID, *got.Link.UID)
			assert.Equal(t, "https://example.com", *got.Link.URL)
			assert.Equal(t, "9", *got.Etag)
		})
	}
}

func TestDeleteProjectLink(t *testing.T) {
	tests := []struct {
		name       string
		ifMatch    *string
		setupMocks func(*resourceMocks)
		wantErr    any
	}{
		{
			name:    "success deletes at the If-Match revision",
			ifMatch: misc.StringPtr("4"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("DeleteLink", mock.Anything, testProjectUID, testLinkUID, uint64(4)).Return(nil)
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
			ifMatch: misc.StringPtr("4"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("DeleteLink", mock.Anything, testProjectUID, testLinkUID, uint64(4)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: &projsvc.ConflictError{},
		},
		{
			name:    "link not found",
			ifMatch: misc.StringPtr("4"),
			setupMocks: func(m *resourceMocks) {
				m.link.On("DeleteLink", mock.Anything, testProjectUID, testLinkUID, uint64(4)).Return(domain.ErrLinkNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, m := setupResourceAPI()
			tt.setupMocks(m)

			err := api.DeleteProjectLink(context.Background(), &projsvc.DeleteProjectLinkPayload{
				UID: testProjectUID, LinkUID: testLinkUID, IfMatch: tt.ifMatch,
			})

			if tt.wantErr != nil {
				assert.IsType(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			m.link.AssertExpectations(t)
		})
	}
}
