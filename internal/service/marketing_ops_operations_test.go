// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"testing"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestProjectsService_AddMarketingOpsMember(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		username   string
		setupMocks func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		notReady   bool
		wantErr    error
	}{
		{
			name:       "success",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				projectDB := &models.ProjectBase{UID: "project-uid-1", Slug: "test-project", Name: "Test Project"}
				settingsDB := &models.ProjectSettings{UID: "project-uid-1"}
				mockRepo.On("GetProjectBase", mock.Anything, "project-uid-1").Return(projectDB, nil)
				mockRepo.On("GetProjectSettings", mock.Anything, "project-uid-1").Return(settingsDB, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberPutSubject, fgatypes.GenericFGAMessage{
					ObjectType: "team",
					Operation:  "member_put",
					Data: fgatypes.GenericMemberData{
						UID:       "marketing-ops-project-uid-1",
						Username:  "bob-fixture",
						Relations: []string{fgaconstants.RelationMember},
					},
				}).Return(nil)
			},
		},
		{
			name:       "empty username",
			projectUID: "project-uid-1",
			username:   "",
			setupMocks: func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "project not found",
			projectUID: "missing",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBase", mock.Anything, "missing").Return(nil, domain.ErrProjectNotFound)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "get project base error",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				mockRepo.On("GetProjectBase", mock.Anything, "project-uid-1").Return(nil, domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "get project settings error",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				projectDB := &models.ProjectBase{UID: "project-uid-1"}
				mockRepo.On("GetProjectBase", mock.Anything, "project-uid-1").Return(projectDB, nil)
				mockRepo.On("GetProjectSettings", mock.Anything, "project-uid-1").Return(nil, domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "publish failure",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				projectDB := &models.ProjectBase{UID: "project-uid-1"}
				settingsDB := &models.ProjectSettings{UID: "project-uid-1"}
				mockRepo.On("GetProjectBase", mock.Anything, "project-uid-1").Return(projectDB, nil)
				mockRepo.On("GetProjectSettings", mock.Anything, "project-uid-1").Return(settingsDB, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberPutSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "service not ready",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {},
			notReady:   true,
			wantErr:    domain.ErrServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, _ := setupServiceForTesting()
			if tt.notReady {
				service.ProjectRepository = nil
			}
			tt.setupMocks(mockRepo, mockBuilder)

			err := service.AddMarketingOpsMember(t.Context(), tt.projectUID, tt.username)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
		})
	}
}

func TestProjectsService_RemoveMarketingOpsMember(t *testing.T) {
	tests := []struct {
		name       string
		projectUID string
		username   string
		setupMocks func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		notReady   bool
		wantErr    error
	}{
		{
			name:       "success",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "project-uid-1").Return(true, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberRemoveSubject, fgatypes.GenericFGAMessage{
					ObjectType: "team",
					Operation:  "member_remove",
					Data: fgatypes.GenericMemberData{
						UID:       "marketing-ops-project-uid-1",
						Username:  "bob-fixture",
						Relations: []string{fgaconstants.RelationMember},
					},
				}).Return(nil)
			},
		},
		{
			name:       "empty username",
			projectUID: "project-uid-1",
			username:   "",
			setupMocks: func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {},
			wantErr:    domain.ErrValidationFailed,
		},
		{
			name:       "project not found",
			projectUID: "missing",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			wantErr: domain.ErrProjectNotFound,
		},
		{
			name:       "project exists check error",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "project-uid-1").Return(false, domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "publish failure",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "project-uid-1").Return(true, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberRemoveSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(domain.ErrInternal)
			},
			wantErr: domain.ErrInternal,
		},
		{
			name:       "service not ready",
			projectUID: "project-uid-1",
			username:   "bob-fixture",
			setupMocks: func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {},
			notReady:   true,
			wantErr:    domain.ErrServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockRepo, mockBuilder, _ := setupServiceForTesting()
			if tt.notReady {
				service.ProjectRepository = nil
			}
			tt.setupMocks(mockRepo, mockBuilder)

			err := service.RemoveMarketingOpsMember(t.Context(), tt.projectUID, tt.username)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			mockRepo.AssertExpectations(t)
			mockBuilder.AssertExpectations(t)
		})
	}
}
