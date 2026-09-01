// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	projsvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

func TestAddProjectMarketingOpsMember(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.AddProjectMarketingOpsMemberPayload
		setupMocks    func(*domain.MockProjectRepository, *domain.MockMessageBuilder, *domain.MockUserReader)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.AddProjectMarketingOpsMemberPayload{
				UID:      "project-uid-1",
				Username: "bob-fixture",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder, mockUserReader *domain.MockUserReader) {
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "bob-fixture").Return(&domain.UserMetadata{}, nil)
				mockRepo.On("GetProjectBase", mock.Anything, "project-uid-1").Return(&models.ProjectBase{UID: "project-uid-1"}, nil)
				mockRepo.On("GetProjectSettings", mock.Anything, "project-uid-1").Return(&models.ProjectSettings{UID: "project-uid-1"}, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberPutSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
			},
		},
		{
			name: "project not found",
			payload: &projsvc.AddProjectMarketingOpsMemberPayload{
				UID:      "missing",
				Username: "bob-fixture",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder, mockUserReader *domain.MockUserReader) {
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, "bob-fixture").Return(&domain.UserMetadata{}, nil)
				mockRepo.On("GetProjectBase", mock.Anything, "missing").Return(nil, domain.ErrProjectNotFound)
			},
			expectedError: true,
		},
		{
			name: "missing username",
			payload: &projsvc.AddProjectMarketingOpsMemberPayload{
				UID:      "project-uid-1",
				Username: "",
			},
			setupMocks:    func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder, _ *domain.MockUserReader) {},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockBuilder := setupAPI()
			mockUserReader := api.service.UserReader.(*domain.MockUserReader)
			tt.setupMocks(mockRepo, mockBuilder, mockUserReader)

			err := api.AddProjectMarketingOpsMember(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveProjectMarketingOpsMember(t *testing.T) {
	tests := []struct {
		name          string
		payload       *projsvc.RemoveProjectMarketingOpsMemberPayload
		setupMocks    func(*domain.MockProjectRepository, *domain.MockMessageBuilder)
		expectedError bool
	}{
		{
			name: "success",
			payload: &projsvc.RemoveProjectMarketingOpsMemberPayload{
				UID:      "project-uid-1",
				Username: "bob-fixture",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, mockBuilder *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "project-uid-1").Return(true, nil)
				mockBuilder.On("PublishAccessMessage", mock.Anything, fgaconstants.GenericMemberRemoveSubject, mock.AnythingOfType("types.GenericFGAMessage")).Return(nil)
			},
		},
		{
			name: "project not found",
			payload: &projsvc.RemoveProjectMarketingOpsMemberPayload{
				UID:      "missing",
				Username: "bob-fixture",
			},
			setupMocks: func(mockRepo *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {
				mockRepo.On("ProjectExists", mock.Anything, "missing").Return(false, nil)
			},
			expectedError: true,
		},
		{
			name: "missing username",
			payload: &projsvc.RemoveProjectMarketingOpsMemberPayload{
				UID:      "project-uid-1",
				Username: "",
			},
			setupMocks:    func(_ *domain.MockProjectRepository, _ *domain.MockMessageBuilder) {},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, mockRepo, mockBuilder := setupAPI()
			tt.setupMocks(mockRepo, mockBuilder)

			err := api.RemoveProjectMarketingOpsMember(context.Background(), tt.payload)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
