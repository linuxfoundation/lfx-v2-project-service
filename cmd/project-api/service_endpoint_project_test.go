// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"

	projsvchttpclient "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/client"
	projsvchttpserver "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/misc"
)

// setupAPI creates a new ProjectsAPI with mocked dependencies.
func setupAPI() (*ProjectsAPI, *domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder) {
	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	mockRepo := &domainmocks.MockProjectRepository{}
	mockMessageBuilder := &domainmocks.MockMessageBuilder{}
	mockJwtAuth := &auth.MockJWTAuth{}
	mockUserReader := &domainmocks.MockUserReader{}

	projectService := &service.ProjectsService{
		ProjectRepository:  mockRepo,
		DocumentRepository: &domainmocks.MockDocumentRepository{},
		LinkRepository:     &domainmocks.MockLinkRepository{},
		FolderRepository:   &domainmocks.MockFolderRepository{},
		Publisher:          mockMessageBuilder,
		Sender:             mockMessageBuilder,
		Auth:               mockJwtAuth,
		UserReader:         mockUserReader,
		Resolver:           service.NewUserResolver(mockUserReader),
		Dispatcher:         service.NewNotificationDispatcher(mockMessageBuilder, service.NewUserResolver(mockUserReader), false, false),
	}

	api := &ProjectsAPI{
		service: projectService,
		auth:    mockJwtAuth,
	}

	return api, mockRepo, mockMessageBuilder
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		payload        *projsvc.GetProjectsPayload
		setupMocks     func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder)
		expectedError  bool
		expectedLength int
	}{
		{
			name:    "success with projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				// Create mock projects
				now := time.Now()
				projectsBase := []*models.ProjectBase{
					{
						UID:         "project-1",
						Slug:        "test-1",
						Name:        "Test Project 1",
						Description: "Test 1",
						Public:      true,
						CreatedAt:   &now,
						UpdatedAt:   &now,
					},
				}
				projectsSettings := []*models.ProjectSettings{
					{
						UID:      "project-1",
						Writers:  []models.UserInfo{{Username: "user2", Name: "User Two", Email: "user2@example.com", Avatar: ""}},
						Auditors: []models.UserInfo{{Username: "user1", Name: "User One", Email: "user1@example.com", Avatar: ""}},
					},
				}

				mockRepo.On("ListAllProjects", mock.Anything).Return(projectsBase, projectsSettings, nil)
			},
			expectedError:  false,
			expectedLength: 1,
		},
		{
			name:    "success with no projects",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return([]*models.ProjectBase{}, []*models.ProjectSettings{}, nil)
			},
			expectedError:  false,
			expectedLength: 0,
		},
		{
			name:    "repository error",
			payload: &projsvc.GetProjectsPayload{},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("ListAllProjects", mock.Anything).Return(nil, nil, domain.ErrInternal)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			tt.setupMocks(mockRepo, mockMsg)

			result, err := api.GetProjects(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.Projects, tt.expectedLength)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestCreateProject(t *testing.T) {
	tests := []struct {
		name            string
		payload         *projsvc.CreateProjectPayload
		setupMocks      func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder)
		setupUserReader func(*domainmocks.MockUserReader)
		expectedError   bool
	}{
		{
			name: "success",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "test-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      misc.BoolPtr(true),
				ParentUID:   "787620d0-d7de-449a-b0bf-9d28b13da818",
				Writers: []*projsvc.UserInfo{
					{Username: misc.StringPtr("user1"), Name: misc.StringPtr("User One"), Email: misc.StringPtr("user1@example.com"), Avatar: misc.StringPtr("")},
					{Username: misc.StringPtr("user2"), Name: misc.StringPtr("User Two"), Email: misc.StringPtr("user2@example.com"), Avatar: misc.StringPtr("")},
				},
				Auditors: []*projsvc.UserInfo{
					{Username: misc.StringPtr("user3"), Name: misc.StringPtr("User Three"), Email: misc.StringPtr("user3@example.com"), Avatar: misc.StringPtr("")},
					{Username: misc.StringPtr("user4"), Name: misc.StringPtr("User Four"), Email: misc.StringPtr("user4@example.com"), Avatar: misc.StringPtr("")},
				},
			},
			setupUserReader: func(mockUserReader *domainmocks.MockUserReader) {
				mockUserReader.On("UsernameByEmail", mock.Anything, "user1@example.com").Return("user1", nil)
				mockUserReader.On("UsernameByEmail", mock.Anything, "user2@example.com").Return("user2", nil)
				mockUserReader.On("UsernameByEmail", mock.Anything, "user3@example.com").Return("user3", nil)
				mockUserReader.On("UsernameByEmail", mock.Anything, "user4@example.com").Return("user4", nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "user1").Return((*domain.UserMetadata)(nil), nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "user2").Return((*domain.UserMetadata)(nil), nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "user3").Return((*domain.UserMetadata)(nil), nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "user4").Return((*domain.UserMetadata)(nil), nil)
			},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				// Mock parent project exists (for ParentUID validation)
				mockRepo.On("ProjectExists", mock.Anything, "787620d0-d7de-449a-b0bf-9d28b13da818").Return(true, nil)
				// Mock slug doesn't exist
				mockRepo.On("ProjectSlugExists", mock.Anything, "test-project").Return(false, nil)
				// Mock successful project creation — MatchedBy validates enriched LFIDs were persisted.
				mockRepo.On("CreateProject", mock.Anything, mock.AnythingOfType("*models.ProjectBase"),
					mock.MatchedBy(func(s *models.ProjectSettings) bool {
						return len(s.Writers) == 2 && s.Writers[0].Username == "user1" && s.Writers[1].Username == "user2" &&
							len(s.Auditors) == 2 && s.Auditors[0].Username == "user3" && s.Auditors[1].Username == "user4"
					})).Return(nil)
				// Mock message sending
				mockMsg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("types.IndexerMessageEnvelope")).Return(nil).Times(2)
				mockMsg.On("PublishAccessMessage", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
				mockMsg.On("SendProjectEventMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "slug already exists",
			payload: &projsvc.CreateProjectPayload{
				Slug:        "existing-project",
				Name:        "Test Project",
				Description: "Test description",
				Public:      misc.BoolPtr(true),
				ParentUID:   "787620d0-d7de-449a-b0bf-9d28b13da818",
			},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				// Mock slug exists
				mockRepo.On("ProjectSlugExists", mock.Anything, "existing-project").Return(true, nil)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			mockUserReader := api.service.UserReader.(*domainmocks.MockUserReader)
			if tt.setupUserReader != nil {
				tt.setupUserReader(mockUserReader)
			}
			tt.setupMocks(mockRepo, mockMsg)

			result, err := api.CreateProject(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.payload.Slug, *result.Slug)
				assert.Equal(t, tt.payload.Name, *result.Name)
			}

			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
			mockUserReader.AssertExpectations(t)
		})
	}
}

func TestUpdateProjectSettingsHTTPClearsMentorshipAdmins(t *testing.T) {
	var received *projsvc.UpdateProjectSettingsPayload

	mux := goahttp.NewMuxer()
	handler := projsvchttpserver.NewUpdateProjectSettingsHandler(
		func(ctx context.Context, req any) (any, error) {
			payload, ok := req.(*projsvc.UpdateProjectSettingsPayload)
			require.True(t, ok)
			received = payload

			require.NotNil(t, payload.MentorshipProgramAdmins)
			assert.Empty(t, payload.MentorshipProgramAdmins)

			return &projsvc.ProjectSettings{
				UID:                     payload.UID,
				MentorshipProgramAdmins: payload.MentorshipProgramAdmins,
			}, nil
		},
		mux,
		goahttp.RequestDecoder,
		goahttp.ResponseEncoder,
		nil,
		nil,
	)
	projsvchttpserver.MountUpdateProjectSettingsHandler(mux, handler)

	server := httptest.NewServer(mux)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	client := projsvchttpclient.NewClient(serverURL.Scheme, serverURL.Host, server.Client(), goahttp.RequestEncoder, goahttp.ResponseDecoder, false)
	version := "1"
	uid := "787620d0-d7de-449a-b0bf-9d28b13da818"
	ifMatch := "1"

	_, err = client.UpdateProjectSettings()(context.Background(), &projsvc.UpdateProjectSettingsPayload{
		UID:                     &uid,
		Version:                 &version,
		IfMatch:                 &ifMatch,
		MentorshipProgramAdmins: []*projsvc.UserInfo{},
	})
	require.NoError(t, err)
	require.NotNil(t, received)
	require.NotNil(t, received.MentorshipProgramAdmins)
	assert.Empty(t, received.MentorshipProgramAdmins)
}

func TestResolveProjectSlug(t *testing.T) {
	tests := []struct {
		name            string
		payload         *projsvc.ResolveProjectSlugPayload
		setupMocks      func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder)
		expectedError   bool
		expectedErrType error
		expectedUID     string
	}{
		{
			name:    "success",
			payload: &projsvc.ResolveProjectSlugPayload{Slug: "test-project"},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "test-project").
					Return("787620d0-d7de-449a-b0bf-9d28b13da818", nil)
			},
			expectedError: false,
			expectedUID:   "787620d0-d7de-449a-b0bf-9d28b13da818",
		},
		{
			name:    "not found maps to 404",
			payload: &projsvc.ResolveProjectSlugPayload{Slug: "nonexistent-slug"},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "nonexistent-slug").
					Return("", domain.ErrProjectNotFound)
			},
			expectedError:   true,
			expectedErrType: &projsvc.NotFoundError{},
		},
		{
			name:    "repository error maps to 500",
			payload: &projsvc.ResolveProjectSlugPayload{Slug: "test-project"},
			setupMocks: func(mockRepo *domainmocks.MockProjectRepository, mockMsg *domainmocks.MockMessageBuilder) {
				mockRepo.On("GetProjectUIDFromSlug", mock.Anything, "test-project").
					Return("", domain.ErrInternal)
			},
			expectedError:   true,
			expectedErrType: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			tt.setupMocks(mockRepo, mockMsg)

			result, err := api.ResolveProjectSlug(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.IsType(t, tt.expectedErrType, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedUID, result.UID)
			}

			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}

func TestGetOneProjectBase(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		payload    *projsvc.GetOneProjectBasePayload
		setupMocks func(*domainmocks.MockProjectRepository)
		wantErr    any
	}{
		{
			name:    "success returns the project and etag",
			payload: &projsvc.GetOneProjectBasePayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("GetProjectBaseWithRevision", mock.Anything, uid).Return(&models.ProjectBase{UID: uid, Slug: "test-project", Name: "Test Project"}, uint64(4), nil)
			},
		},
		{
			name:       "missing uid is a bad request",
			payload:    &projsvc.GetOneProjectBasePayload{},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "not found",
			payload: &projsvc.GetOneProjectBasePayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("GetProjectBaseWithRevision", mock.Anything, uid).Return(nil, uint64(0), domain.ErrProjectNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "repository failure is an internal error",
			payload: &projsvc.GetOneProjectBasePayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("GetProjectBaseWithRevision", mock.Anything, uid).Return(nil, uint64(0), errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, _ := setupAPI()
			tt.setupMocks(mockRepo)

			got, err := api.GetOneProjectBase(context.Background(), tt.payload)

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, uid, *got.Project.UID)
			assert.Equal(t, "Test Project", *got.Project.Name)
			assert.Equal(t, "4", *got.Etag)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestGetOneProjectSettings(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		payload    *projsvc.GetOneProjectSettingsPayload
		setupMocks func(*domainmocks.MockProjectRepository)
		wantErr    any
	}{
		{
			name:    "success returns the settings and etag",
			payload: &projsvc.GetOneProjectSettingsPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(true, nil)
				r.On("GetProjectSettingsWithRevision", mock.Anything, uid).Return(&models.ProjectSettings{UID: uid}, uint64(8), nil)
			},
		},
		{
			name:       "missing uid is a bad request",
			payload:    &projsvc.GetOneProjectSettingsPayload{},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.GetOneProjectSettingsPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(false, nil)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "existence check failure is an internal error",
			payload: &projsvc.GetOneProjectSettingsPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(false, errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
		{
			name:    "settings not found",
			payload: &projsvc.GetOneProjectSettingsPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(true, nil)
				r.On("GetProjectSettingsWithRevision", mock.Anything, uid).Return(nil, uint64(0), domain.ErrProjectNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, _ := setupAPI()
			tt.setupMocks(mockRepo)

			got, err := api.GetOneProjectSettings(context.Background(), tt.payload)

			if tt.wantErr != nil {
				require.IsType(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, uid, *got.ProjectSettings.UID)
			assert.Equal(t, "8", *got.Etag)
			mockRepo.AssertExpectations(t)
		})
	}
}

// TestUpdateProjectBase covers the adapter's request validation and error
// mapping. The full update flow (FGA, indexing, notifications) is covered by
// the service-layer tests.
func TestUpdateProjectBase(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		payload    *projsvc.UpdateProjectBasePayload
		setupMocks func(*domainmocks.MockProjectRepository)
		wantErr    any
	}{
		{
			name:       "missing uid is a bad request",
			payload:    &projsvc.UpdateProjectBasePayload{Name: "Test Project", IfMatch: misc.StringPtr("1")},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "blank name is a bad request",
			payload:    &projsvc.UpdateProjectBasePayload{UID: misc.StringPtr(uid), Name: "   ", IfMatch: misc.StringPtr("1")},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "archived project without a dissolution date is a bad request",
			payload:    &projsvc.UpdateProjectBasePayload{UID: misc.StringPtr(uid), Name: "Test Project", Stage: misc.StringPtr("Archived"), IfMatch: misc.StringPtr("1")},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "missing If-Match is a bad request",
			payload:    &projsvc.UpdateProjectBasePayload{UID: misc.StringPtr(uid), Name: "Test Project"},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.UpdateProjectBasePayload{UID: misc.StringPtr(uid), Name: "Test Project", IfMatch: misc.StringPtr("1")},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("GetProjectBase", mock.Anything, uid).Return(nil, domain.ErrProjectNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, _ := setupAPI()
			tt.setupMocks(mockRepo)

			got, err := api.UpdateProjectBase(context.Background(), tt.payload)

			require.IsType(t, tt.wantErr, err)
			assert.Nil(t, got)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestDeleteProject(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		payload    *projsvc.DeleteProjectPayload
		setupMocks func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder)
		wantErr    any
	}{
		{
			name:    "success deletes at the If-Match revision",
			payload: &projsvc.DeleteProjectPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("2")},
			setupMocks: func(r *domainmocks.MockProjectRepository, msg *domainmocks.MockMessageBuilder) {
				r.On("GetProjectBase", mock.Anything, uid).Return(&models.ProjectBase{UID: uid, FundingModel: []string{"Crowdfunding"}}, nil)
				r.On("DeleteProject", mock.Anything, uid, uint64(2)).Return(nil)
				msg.On("SendIndexerMessage", mock.Anything, mock.AnythingOfType("string"), uid).Return(nil)
				msg.On("PublishAccessMessage", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
			},
		},
		{
			name:       "missing uid is a bad request",
			payload:    &projsvc.DeleteProjectPayload{IfMatch: misc.StringPtr("2")},
			setupMocks: func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "missing If-Match is a bad request",
			payload:    &projsvc.DeleteProjectPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(*domainmocks.MockProjectRepository, *domainmocks.MockMessageBuilder) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.DeleteProjectPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("2")},
			setupMocks: func(r *domainmocks.MockProjectRepository, _ *domainmocks.MockMessageBuilder) {
				r.On("GetProjectBase", mock.Anything, uid).Return(nil, domain.ErrProjectNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "non-crowdfunding project cannot be deleted",
			payload: &projsvc.DeleteProjectPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("2")},
			setupMocks: func(r *domainmocks.MockProjectRepository, _ *domainmocks.MockMessageBuilder) {
				r.On("GetProjectBase", mock.Anything, uid).Return(&models.ProjectBase{UID: uid, FundingModel: []string{"Membership"}}, nil)
			},
			wantErr: &projsvc.BadRequestError{},
		},
		{
			name:    "revision mismatch is a conflict",
			payload: &projsvc.DeleteProjectPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("2")},
			setupMocks: func(r *domainmocks.MockProjectRepository, _ *domainmocks.MockMessageBuilder) {
				r.On("GetProjectBase", mock.Anything, uid).Return(&models.ProjectBase{UID: uid, FundingModel: []string{"Crowdfunding"}}, nil)
				r.On("DeleteProject", mock.Anything, uid, uint64(2)).Return(domain.ErrRevisionMismatch)
			},
			wantErr: &projsvc.ConflictError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockMsg := setupAPI()
			tt.setupMocks(mockRepo, mockMsg)

			err := api.DeleteProject(context.Background(), tt.payload)

			if tt.wantErr != nil {
				assert.IsType(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}
}

// TestUpdateProjectSettings covers the adapter's request validation and error
// mapping. The full update flow (FGA, indexing, notifications) is covered by
// the service-layer tests.
func TestUpdateProjectSettings(t *testing.T) {
	const uid = "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name       string
		payload    *projsvc.UpdateProjectSettingsPayload
		setupMocks func(*domainmocks.MockProjectRepository)
		wantErr    any
	}{
		{
			name:       "missing uid is a bad request",
			payload:    &projsvc.UpdateProjectSettingsPayload{IfMatch: misc.StringPtr("1")},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:       "missing If-Match is a bad request",
			payload:    &projsvc.UpdateProjectSettingsPayload{UID: misc.StringPtr(uid)},
			setupMocks: func(*domainmocks.MockProjectRepository) {},
			wantErr:    &projsvc.BadRequestError{},
		},
		{
			name:    "project not found",
			payload: &projsvc.UpdateProjectSettingsPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("1")},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(false, nil)
			},
			wantErr: &projsvc.NotFoundError{},
		},
		{
			name:    "existence check failure is an internal error",
			payload: &projsvc.UpdateProjectSettingsPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("1")},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(false, errors.New("kv unavailable"))
			},
			wantErr: &projsvc.InternalServerError{},
		},
		{
			name:    "settings not found",
			payload: &projsvc.UpdateProjectSettingsPayload{UID: misc.StringPtr(uid), IfMatch: misc.StringPtr("1")},
			setupMocks: func(r *domainmocks.MockProjectRepository) {
				r.On("ProjectExists", mock.Anything, uid).Return(true, nil)
				r.On("GetProjectSettings", mock.Anything, uid).Return(nil, domain.ErrProjectNotFound)
			},
			wantErr: &projsvc.NotFoundError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, _ := setupAPI()
			tt.setupMocks(mockRepo)

			got, err := api.UpdateProjectSettings(context.Background(), tt.payload)

			require.IsType(t, tt.wantErr, err)
			assert.Nil(t, got)
			mockRepo.AssertExpectations(t)
		})
	}
}
