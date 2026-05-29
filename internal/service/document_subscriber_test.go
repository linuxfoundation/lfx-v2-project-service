// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"
	"testing"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

func TestHandleProjectDocumentCreated(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noLFID := models.UserInfo{Email: "nolfid@example.com", Name: "No LFID"}
	noEmail := models.UserInfo{Username: "noemail", Name: "No Email"}

	baseSettings := func(writers, auditors []models.UserInfo) *models.ProjectSettings {
		return &models.ProjectSettings{UID: "proj-1", Writers: writers, Auditors: auditors}
	}

	folderUID := "folder-1"

	tests := []struct {
		name           string
		doc            models.ProjectDocument
		settings       *models.ProjectSettings
		wantEmailCount int
		emailsEnabled  bool
		folderName     string // expected folder name in email; set only when doc has a FolderUID
	}{
		{
			name:           "notifies writer and auditor",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Charter", FileName: "charter.pdf", UploadedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name:           "includes folder name when document is in a folder",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Charter", FileName: "charter.pdf", UploadedByUsername: "uploader", FolderUID: &folderUID},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{}),
			wantEmailCount: 1,
			emailsEnabled:  true,
			folderName:     "Legal",
		},
		{
			name:           "no-LFID users skipped",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Doc", UploadedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{noLFID}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name:           "users without email skipped",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Doc", UploadedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{noEmail}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name:           "duplicate in writer and auditor deduplicated",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Doc", UploadedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{writer}),
			wantEmailCount: 1,
			emailsEnabled:  true,
		},
		{
			name:           "EmailsEnabled=false — no emails sent",
			doc:            models.ProjectDocument{ProjectUID: "proj-1", Name: "Doc", UploadedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 0,
			emailsEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domain.MockProjectRepository{}
			mockMsg := &domain.MockMessageBuilder{}
			mockUserReader := &domain.MockUserReader{}
			mockFolder := &domain.MockFolderRepository{}

			if tt.emailsEnabled && tt.settings != nil {
				mockRepo.On("GetProjectBase", mock.Anything, tt.doc.ProjectUID).
					Return(makeProjectBase(tt.doc.ProjectUID, "Demo Project", "demo-project"), nil)
				mockRepo.On("GetProjectSettings", mock.Anything, tt.doc.ProjectUID).
					Return(tt.settings, nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
					Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
				if tt.doc.FolderUID != nil {
					mockFolder.On("GetFolder", mock.Anything, tt.doc.ProjectUID, *tt.doc.FolderUID).
						Return(&models.ProjectFolder{UID: *tt.doc.FolderUID, Name: tt.folderName}, uint64(1), nil)
				}
			}

			if tt.wantEmailCount > 0 {
				if tt.folderName != "" {
					mockMsg.On("SendEmailRequest", mock.Anything, mock.MatchedBy(func(req emailapi.SendEmailRequest) bool {
						return strings.Contains(req.HTML, tt.folderName)
					})).Return(nil).Times(tt.wantEmailCount)
				} else {
					mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
						Return(nil).Times(tt.wantEmailCount)
				}
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				FolderRepository:  mockFolder,
				MessageBuilder:    mockMsg,
				UserReader:        mockUserReader,
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
					EmailsEnabled:       tt.emailsEnabled,
				},
			}

			msg := domain.NewMockMessage(marshalEvent(t, tt.doc), "")
			err := svc.HandleProjectDocumentCreated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil", func(t *testing.T) {
		svc := &ProjectsService{Config: ServiceConfig{EmailsEnabled: true}}
		msg := domain.NewMockMessage([]byte("not json"), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("send failure swallowed — returns nil", func(t *testing.T) {
		mockRepo := &domain.MockProjectRepository{}
		mockMsg := &domain.MockMessageBuilder{}
		mockUserReader := &domain.MockUserReader{}

		doc := models.ProjectDocument{ProjectUID: "proj-1", Name: "Doc", FileName: "doc.pdf", UploadedByUsername: "uploader"}
		settings := baseSettings([]models.UserInfo{writer}, []models.UserInfo{})

		mockRepo.On("GetProjectBase", mock.Anything, "proj-1").
			Return(makeProjectBase("proj-1", "Demo Project", "demo-project"), nil)
		mockRepo.On("GetProjectSettings", mock.Anything, "proj-1").
			Return(settings, nil)
		mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
			Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
		mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
			Return(assert.AnError)

		svc := &ProjectsService{
			ProjectRepository: mockRepo,
			FolderRepository:  &domain.MockFolderRepository{},
			MessageBuilder:    mockMsg,
			UserReader:        mockUserReader,
			Config:            ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domain.NewMockMessage(marshalEvent(t, doc), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("SendEmailRequest called with correct To field", func(t *testing.T) {
		mockRepo := &domain.MockProjectRepository{}
		mockMsg := &domain.MockMessageBuilder{}
		mockUserReader := &domain.MockUserReader{}

		doc := models.ProjectDocument{ProjectUID: "proj-1", Name: "Spec", FileName: "spec.pdf", UploadedByUsername: "uploader"}
		settings := baseSettings([]models.UserInfo{writer}, []models.UserInfo{})

		mockRepo.On("GetProjectBase", mock.Anything, "proj-1").
			Return(makeProjectBase("proj-1", "Demo Project", "demo-project"), nil)
		mockRepo.On("GetProjectSettings", mock.Anything, "proj-1").
			Return(settings, nil)
		mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
			Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
		mockMsg.On("SendEmailRequest", mock.Anything, mock.MatchedBy(func(req emailapi.SendEmailRequest) bool {
			return req.To == writer.Email
		})).Return(nil).Once()

		svc := &ProjectsService{
			ProjectRepository: mockRepo,
			FolderRepository:  &domain.MockFolderRepository{},
			MessageBuilder:    mockMsg,
			UserReader:        mockUserReader,
			Config:            ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domain.NewMockMessage(marshalEvent(t, doc), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
		mockMsg.AssertExpectations(t)
	})
}

func TestHandleProjectLinkCreated(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}

	baseSettings := func(writers, auditors []models.UserInfo) *models.ProjectSettings {
		return &models.ProjectSettings{UID: "proj-1", Writers: writers, Auditors: auditors}
	}

	tests := []struct {
		name           string
		link           models.ProjectLink
		settings       *models.ProjectSettings
		wantEmailCount int
		emailsEnabled  bool
	}{
		{
			name:           "notifies writer and auditor",
			link:           models.ProjectLink{ProjectUID: "proj-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name:           "EmailsEnabled=false — no emails sent",
			link:           models.ProjectLink{ProjectUID: "proj-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedByUsername: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 0,
			emailsEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domain.MockProjectRepository{}
			mockMsg := &domain.MockMessageBuilder{}
			mockUserReader := &domain.MockUserReader{}

			if tt.emailsEnabled && tt.settings != nil {
				mockRepo.On("GetProjectBase", mock.Anything, tt.link.ProjectUID).
					Return(makeProjectBase(tt.link.ProjectUID, "Demo Project", "demo-project"), nil)
				mockRepo.On("GetProjectSettings", mock.Anything, tt.link.ProjectUID).
					Return(tt.settings, nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
					Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
			}

			if tt.wantEmailCount > 0 {
				mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
					Return(nil).Times(tt.wantEmailCount)
			}

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				FolderRepository:  &domain.MockFolderRepository{},
				MessageBuilder:    mockMsg,
				UserReader:        mockUserReader,
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
					EmailsEnabled:       tt.emailsEnabled,
				},
			}

			msg := domain.NewMockMessage(marshalEvent(t, tt.link), "")
			err := svc.HandleProjectLinkCreated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil", func(t *testing.T) {
		svc := &ProjectsService{Config: ServiceConfig{EmailsEnabled: true}}
		msg := domain.NewMockMessage([]byte("not json"), "")
		err := svc.HandleProjectLinkCreated(context.Background(), msg)
		assert.NoError(t, err)
	})
}

func TestCollectDocumentRecipients(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noLFID := models.UserInfo{Email: "nolfid@example.com"}
	noEmail := models.UserInfo{Username: "noemail"}

	t.Run("no eligible recipients returns empty", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{noLFID, noEmail},
			Auditors: []models.UserInfo{},
		}
		assert.Empty(t, collectDocumentRecipients(settings, ""))
	})

	t.Run("writer and auditor both included", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{writer},
			Auditors: []models.UserInfo{auditor},
		}
		result := collectDocumentRecipients(settings, "someone-else")
		assert.Len(t, result, 2)
	})

	t.Run("uploader excluded", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{writer},
			Auditors: []models.UserInfo{auditor},
		}
		result := collectDocumentRecipients(settings, writer.Username)
		assert.Len(t, result, 1)
		assert.Equal(t, auditor.Username, result[0].Username)
	})

	t.Run("duplicate username deduplicated", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{writer},
			Auditors: []models.UserInfo{writer},
		}
		assert.Len(t, collectDocumentRecipients(settings, ""), 1)
	})
}
