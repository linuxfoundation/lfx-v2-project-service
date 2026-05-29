// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"testing"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

func TestHandleDocumentUploaded(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noLFID := models.UserInfo{Email: "nolfid@example.com", Name: "No LFID"}
	noEmail := models.UserInfo{Username: "noemail", Name: "No Email"}

	baseSettings := func(writers, auditors []models.UserInfo) *models.ProjectSettings {
		return &models.ProjectSettings{
			UID:      "proj-1",
			Writers:  writers,
			Auditors: auditors,
		}
	}

	tests := []struct {
		name           string
		event          events.DocumentUploadedMessage
		settings       *models.ProjectSettings
		wantEmailCount int
		emailsEnabled  bool
	}{
		{
			name: "file upload notifies writer and auditor",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Charter",
				DocumentType: "file",
				FileName:     "charter.pdf",
				Actor:        events.Actor{Username: "uploader"},
			},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name: "link upload notifies writer and auditor",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Spec Link",
				DocumentType: "link",
				URL:          "https://specs.example.com",
				Actor:        events.Actor{Username: "uploader"},
			},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name: "uploader excluded from recipients",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Report",
				DocumentType: "file",
				FileName:     "report.pdf",
				Actor:        events.Actor{Username: writer.Username},
			},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 1, // only auditor; writer is the uploader
			emailsEnabled:  true,
		},
		{
			name: "no-LFID users skipped",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Doc",
				DocumentType: "file",
				Actor:        events.Actor{Username: "someone"},
			},
			settings:       baseSettings([]models.UserInfo{noLFID}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name: "users without email skipped",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Doc",
				DocumentType: "file",
				Actor:        events.Actor{Username: "someone"},
			},
			settings:       baseSettings([]models.UserInfo{noEmail}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name: "duplicate user in writer and auditor deduplicated",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Doc",
				DocumentType: "file",
				Actor:        events.Actor{Username: "someone"},
			},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{writer}),
			wantEmailCount: 1,
			emailsEnabled:  true,
		},
		{
			name: "EmailsEnabled=false — no emails sent",
			event: events.DocumentUploadedMessage{
				ProjectUID:   "proj-1",
				DocumentName: "Doc",
				DocumentType: "file",
				Actor:        events.Actor{Username: "uploader"},
			},
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
				mockRepo.On("GetProjectBase", mock.Anything, tt.event.ProjectUID).
					Return(makeProjectBase(tt.event.ProjectUID, "Demo Project", "demo-project"), nil)
				mockRepo.On("GetProjectSettings", mock.Anything, tt.event.ProjectUID).
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
				MessageBuilder:    mockMsg,
				UserReader:        mockUserReader,
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
					EmailsEnabled:       tt.emailsEnabled,
				},
			}

			msg := domain.NewMockMessage(marshalEvent(t, tt.event), "")
			err := svc.HandleDocumentUploaded(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil without panic", func(t *testing.T) {
		svc := &ProjectsService{Config: ServiceConfig{EmailsEnabled: true}}
		msg := domain.NewMockMessage([]byte("not json"), "")
		err := svc.HandleDocumentUploaded(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("send failure swallowed — returns nil", func(t *testing.T) {
		mockRepo := &domain.MockProjectRepository{}
		mockMsg := &domain.MockMessageBuilder{}
		mockUserReader := &domain.MockUserReader{}

		event := events.DocumentUploadedMessage{
			ProjectUID:   "proj-1",
			DocumentName: "Doc",
			DocumentType: "file",
			Actor:        events.Actor{Username: "uploader"},
		}
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
			MessageBuilder:    mockMsg,
			UserReader:        mockUserReader,
			Config:            ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domain.NewMockMessage(marshalEvent(t, event), "")
		err := svc.HandleDocumentUploaded(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("collectDocumentRecipients — no eligible recipients returns empty", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{noLFID, noEmail},
			Auditors: []models.UserInfo{},
		}
		result := collectDocumentRecipients(settings, "")
		assert.Empty(t, result)
	})

	t.Run("collectDocumentRecipients — writer and auditor both included", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{writer},
			Auditors: []models.UserInfo{auditor},
		}
		result := collectDocumentRecipients(settings, "someone-else")
		assert.Len(t, result, 2)
	})

	t.Run("collectDocumentRecipients — uploader excluded", func(t *testing.T) {
		settings := &models.ProjectSettings{
			Writers:  []models.UserInfo{writer},
			Auditors: []models.UserInfo{auditor},
		}
		result := collectDocumentRecipients(settings, writer.Username)
		assert.Len(t, result, 1)
		assert.Equal(t, auditor.Username, result[0].Username)
	})

	t.Run("collectDocumentRecipients — SendEmailRequest called with correct To field", func(t *testing.T) {
		mockRepo := &domain.MockProjectRepository{}
		mockMsg := &domain.MockMessageBuilder{}
		mockUserReader := &domain.MockUserReader{}

		event := events.DocumentUploadedMessage{
			ProjectUID:   "proj-1",
			DocumentName: "Spec",
			DocumentType: "file",
			FileName:     "spec.pdf",
			Actor:        events.Actor{Username: "uploader"},
		}
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
			MessageBuilder:    mockMsg,
			UserReader:        mockUserReader,
			Config:            ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domain.NewMockMessage(marshalEvent(t, event), "")
		err := svc.HandleDocumentUploaded(context.Background(), msg)
		assert.NoError(t, err)
		mockMsg.AssertExpectations(t)
	})
}
