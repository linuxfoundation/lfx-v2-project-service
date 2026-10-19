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
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

// storedDocumentFor returns the document record a legitimate publisher would have persisted
// before emitting event.
func storedDocumentFor(event events.ProjectDocumentCreatedMessage) *models.ProjectDocument {
	doc := &models.ProjectDocument{
		UID:        event.DocumentUID,
		ProjectUID: event.ProjectUID,
		Name:       event.Name,
		FileName:   event.FileName,
		CreatedBy:  &models.UserInfo{Username: event.CreatedBy},
	}
	if event.FolderUID != "" {
		doc.FolderUID = &event.FolderUID
	}
	return doc
}

// storedLinkFor returns the link record a legitimate publisher would have persisted
// before emitting event.
func storedLinkFor(event events.ProjectLinkCreatedMessage) *models.ProjectLink {
	link := &models.ProjectLink{
		UID:        event.LinkUID,
		ProjectUID: event.ProjectUID,
		Name:       event.Name,
		URL:        event.URL,
		CreatedBy:  &models.UserInfo{Username: event.CreatedBy},
	}
	if event.FolderUID != "" {
		link.FolderUID = &event.FolderUID
	}
	return link
}

func TestHandleProjectDocumentCreated(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noLFID := models.UserInfo{Email: "nolfid@example.com", Name: "No LFID"}
	noEmail := models.UserInfo{Username: "noemail", Name: "No Email"}

	baseSettings := func(writers, auditors []models.UserInfo) *models.ProjectSettings {
		return &models.ProjectSettings{UID: "proj-1", Writers: writers, Auditors: auditors}
	}

	tests := []struct {
		name           string
		event          events.ProjectDocumentCreatedMessage
		settings       *models.ProjectSettings
		wantEmailCount int
		emailsEnabled  bool
		folderName     string
	}{
		{
			name:           "notifies writer and auditor",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Charter", FileName: "charter.pdf", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name:           "uploader excluded from recipients",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Charter", FileName: "charter.pdf", CreatedBy: writer.Username},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 1, // writer is uploader; only auditor receives
			emailsEnabled:  true,
		},
		{
			name:           "includes folder name when document is in a folder",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Charter", FileName: "charter.pdf", FolderUID: "folder-1", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{}),
			wantEmailCount: 1,
			emailsEnabled:  true,
			folderName:     "Legal",
		},
		{
			name:           "no-LFID users skipped",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Doc", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{noLFID}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name:           "users without email skipped",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Doc", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{noEmail}, []models.UserInfo{}),
			wantEmailCount: 0,
			emailsEnabled:  true,
		},
		{
			name:           "duplicate in writer and auditor deduplicated",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Doc", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{writer}),
			wantEmailCount: 1,
			emailsEnabled:  true,
		},
		{
			name:           "EmailsEnabled=false — no emails sent",
			event:          events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Doc", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 0,
			emailsEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domainmocks.MockProjectRepository{}
			mockMsg := &domainmocks.MockMessageBuilder{}
			mockUserReader := &domainmocks.MockUserReader{}
			mockFolder := &domainmocks.MockFolderRepository{}
			mockDocs := &domainmocks.MockDocumentRepository{}

			if tt.emailsEnabled && tt.settings != nil {
				mockDocs.On("GetDocumentMetadata", mock.Anything, tt.event.ProjectUID, tt.event.DocumentUID).
					Return(storedDocumentFor(tt.event), uint64(1), nil)
				mockRepo.On("GetProjectBase", mock.Anything, tt.event.ProjectUID).
					Return(makeProjectBase(tt.event.ProjectUID, "Demo Project", "demo-project"), nil)
				mockRepo.On("GetProjectSettings", mock.Anything, tt.event.ProjectUID).
					Return(tt.settings, nil)
				mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
					Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
				if tt.event.FolderUID != "" {
					mockFolder.On("GetFolder", mock.Anything, tt.event.ProjectUID, tt.event.FolderUID).
						Return(&models.ProjectFolder{UID: tt.event.FolderUID, Name: tt.folderName}, uint64(1), nil)
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
				ProjectRepository:  mockRepo,
				DocumentRepository: mockDocs,
				FolderRepository:   mockFolder,
				Sender:             mockMsg,
				UserReader:         mockUserReader,
				Resolver:           NewUserResolver(mockUserReader),
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
					EmailsEnabled:       tt.emailsEnabled,
				},
			}

			msg := domainmocks.NewMockMessage(marshalEvent(t, tt.event), "")
			err := svc.HandleProjectDocumentCreated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil", func(t *testing.T) {
		svc := &ProjectsService{Config: ServiceConfig{EmailsEnabled: true}}
		msg := domainmocks.NewMockMessage([]byte("not json"), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("send failure swallowed — returns nil", func(t *testing.T) {
		mockRepo := &domainmocks.MockProjectRepository{}
		mockMsg := &domainmocks.MockMessageBuilder{}
		mockUserReader := &domainmocks.MockUserReader{}
		mockDocs := &domainmocks.MockDocumentRepository{}

		event := events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Doc", FileName: "doc.pdf", CreatedBy: "uploader"}
		mockDocs.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(storedDocumentFor(event), uint64(1), nil)
		settings := &models.ProjectSettings{UID: "proj-1", Writers: []models.UserInfo{{Username: "alice", Email: "alice@example.com", Name: "Alice"}}}

		mockRepo.On("GetProjectBase", mock.Anything, "proj-1").
			Return(makeProjectBase("proj-1", "Demo Project", "demo-project"), nil)
		mockRepo.On("GetProjectSettings", mock.Anything, "proj-1").
			Return(settings, nil)
		mockUserReader.On("UserMetadataByPrincipal", mock.Anything, mock.AnythingOfType("string")).
			Return((*domain.UserMetadata)(nil), assert.AnError).Maybe()
		mockMsg.On("SendEmailRequest", mock.Anything, mock.AnythingOfType("api.SendEmailRequest")).
			Return(assert.AnError)

		svc := &ProjectsService{
			ProjectRepository:  mockRepo,
			DocumentRepository: mockDocs,
			FolderRepository:   &domainmocks.MockFolderRepository{},
			Sender:             mockMsg,
			UserReader:         mockUserReader,
			Resolver:           NewUserResolver(mockUserReader),
			Config:             ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domainmocks.NewMockMessage(marshalEvent(t, event), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
	})

	t.Run("SendEmailRequest called with correct To field", func(t *testing.T) {
		mockRepo := &domainmocks.MockProjectRepository{}
		mockMsg := &domainmocks.MockMessageBuilder{}
		mockUserReader := &domainmocks.MockUserReader{}

		mockDocs := &domainmocks.MockDocumentRepository{}

		writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
		event := events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Spec", FileName: "spec.pdf", CreatedBy: "uploader"}
		mockDocs.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(storedDocumentFor(event), uint64(1), nil)
		settings := &models.ProjectSettings{UID: "proj-1", Writers: []models.UserInfo{writer}}

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
			ProjectRepository:  mockRepo,
			DocumentRepository: mockDocs,
			FolderRepository:   &domainmocks.MockFolderRepository{},
			Sender:             mockMsg,
			UserReader:         mockUserReader,
			Resolver:           NewUserResolver(mockUserReader),
			Config:             ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
		}

		msg := domainmocks.NewMockMessage(marshalEvent(t, event), "")
		err := svc.HandleProjectDocumentCreated(context.Background(), msg)
		assert.NoError(t, err)
		mockMsg.AssertExpectations(t)
	})

	forgedDocTests := []struct {
		name   string
		stored func(events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error)
		mutate func(*events.ProjectDocumentCreatedMessage)
	}{
		{
			name: "no stored document — dropped",
			stored: func(events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return nil, domain.ErrDocumentNotFound
			},
			mutate: func(*events.ProjectDocumentCreatedMessage) {},
		},
		{
			name: "repository error — dropped",
			stored: func(events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return nil, assert.AnError
			},
			mutate: func(*events.ProjectDocumentCreatedMessage) {},
		},
		{
			name: "forged name — dropped",
			stored: func(e events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return storedDocumentFor(e), nil
			},
			mutate: func(e *events.ProjectDocumentCreatedMessage) { e.Name = "Urgent: re-authenticate" },
		},
		{
			name: "forged file name — dropped",
			stored: func(e events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return storedDocumentFor(e), nil
			},
			mutate: func(e *events.ProjectDocumentCreatedMessage) { e.FileName = "payload.exe" },
		},
		{
			name: "forged created_by — dropped",
			stored: func(e events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return storedDocumentFor(e), nil
			},
			mutate: func(e *events.ProjectDocumentCreatedMessage) { e.CreatedBy = "executive-director" },
		},
		{
			name: "forged folder_uid — dropped",
			stored: func(e events.ProjectDocumentCreatedMessage) (*models.ProjectDocument, error) {
				return storedDocumentFor(e), nil
			},
			mutate: func(e *events.ProjectDocumentCreatedMessage) { e.FolderUID = "folder-x" },
		},
	}
	for _, tt := range forgedDocTests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domainmocks.MockProjectRepository{}
			mockMsg := &domainmocks.MockMessageBuilder{}
			mockDocs := &domainmocks.MockDocumentRepository{}

			genuine := events.ProjectDocumentCreatedMessage{ProjectUID: "proj-1", DocumentUID: "doc-1", Name: "Charter", FileName: "charter.pdf", CreatedBy: "uploader"}
			doc, err := tt.stored(genuine)
			mockDocs.On("GetDocumentMetadata", mock.Anything, "proj-1", "doc-1").Return(doc, uint64(1), err)

			forged := genuine
			tt.mutate(&forged)

			svc := &ProjectsService{
				ProjectRepository:  mockRepo,
				DocumentRepository: mockDocs,
				FolderRepository:   &domainmocks.MockFolderRepository{},
				Sender:             mockMsg,
				Config:             ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
			}

			msg := domainmocks.NewMockMessage(marshalEvent(t, forged), "")
			assert.NoError(t, svc.HandleProjectDocumentCreated(context.Background(), msg))

			// No project/roster lookup and no email: the event never reaches the fan-out.
			mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, mock.Anything)
			mockRepo.AssertNotCalled(t, "GetProjectSettings", mock.Anything, mock.Anything)
			mockMsg.AssertNotCalled(t, "SendEmailRequest", mock.Anything, mock.Anything)
		})
	}
}

func TestHandleProjectLinkCreated(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}

	baseSettings := func(writers, auditors []models.UserInfo) *models.ProjectSettings {
		return &models.ProjectSettings{UID: "proj-1", Writers: writers, Auditors: auditors}
	}

	tests := []struct {
		name           string
		event          events.ProjectLinkCreatedMessage
		settings       *models.ProjectSettings
		wantEmailCount int
		emailsEnabled  bool
	}{
		{
			name:           "notifies writer and auditor",
			event:          events.ProjectLinkCreatedMessage{ProjectUID: "proj-1", LinkUID: "link-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 2,
			emailsEnabled:  true,
		},
		{
			name:           "uploader excluded from recipients",
			event:          events.ProjectLinkCreatedMessage{ProjectUID: "proj-1", LinkUID: "link-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedBy: writer.Username},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 1, // writer is creator; only auditor receives
			emailsEnabled:  true,
		},
		{
			name:           "EmailsEnabled=false — no emails sent",
			event:          events.ProjectLinkCreatedMessage{ProjectUID: "proj-1", LinkUID: "link-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedBy: "uploader"},
			settings:       baseSettings([]models.UserInfo{writer}, []models.UserInfo{auditor}),
			wantEmailCount: 0,
			emailsEnabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domainmocks.MockProjectRepository{}
			mockMsg := &domainmocks.MockMessageBuilder{}
			mockUserReader := &domainmocks.MockUserReader{}
			mockLinks := &domainmocks.MockLinkRepository{}

			if tt.emailsEnabled && tt.settings != nil {
				mockLinks.On("GetLink", mock.Anything, tt.event.ProjectUID, tt.event.LinkUID).
					Return(storedLinkFor(tt.event), uint64(1), nil)
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
				LinkRepository:    mockLinks,
				FolderRepository:  &domainmocks.MockFolderRepository{},
				Sender:            mockMsg,
				UserReader:        mockUserReader,
				Resolver:          NewUserResolver(mockUserReader),
				Config: ServiceConfig{
					LFXSelfServeBaseURL: "https://app.dev.lfx.dev",
					EmailsEnabled:       tt.emailsEnabled,
				},
			}

			msg := domainmocks.NewMockMessage(marshalEvent(t, tt.event), "")
			err := svc.HandleProjectLinkCreated(context.Background(), msg)
			assert.NoError(t, err)

			mockMsg.AssertNumberOfCalls(t, "SendEmailRequest", tt.wantEmailCount)
			mockRepo.AssertExpectations(t)
			mockMsg.AssertExpectations(t)
		})
	}

	t.Run("invalid JSON — returns nil", func(t *testing.T) {
		svc := &ProjectsService{Config: ServiceConfig{EmailsEnabled: true}}
		msg := domainmocks.NewMockMessage([]byte("not json"), "")
		err := svc.HandleProjectLinkCreated(context.Background(), msg)
		assert.NoError(t, err)
	})

	forgedLinkTests := []struct {
		name   string
		stored func(events.ProjectLinkCreatedMessage) (*models.ProjectLink, error)
		mutate func(*events.ProjectLinkCreatedMessage)
	}{
		{
			name: "no stored link — dropped",
			stored: func(events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) {
				return nil, domain.ErrLinkNotFound
			},
			mutate: func(*events.ProjectLinkCreatedMessage) {},
		},
		{
			name:   "repository error — dropped",
			stored: func(events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) { return nil, assert.AnError },
			mutate: func(*events.ProjectLinkCreatedMessage) {},
		},
		{
			name:   "forged url — dropped",
			stored: func(e events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) { return storedLinkFor(e), nil },
			mutate: func(e *events.ProjectLinkCreatedMessage) { e.URL = "https://attacker.invalid/login" },
		},
		{
			name:   "forged name — dropped",
			stored: func(e events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) { return storedLinkFor(e), nil },
			mutate: func(e *events.ProjectLinkCreatedMessage) { e.Name = "Urgent: re-authenticate" },
		},
		{
			name:   "forged created_by — dropped",
			stored: func(e events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) { return storedLinkFor(e), nil },
			mutate: func(e *events.ProjectLinkCreatedMessage) { e.CreatedBy = "executive-director" },
		},
		{
			name:   "forged folder_uid — dropped",
			stored: func(e events.ProjectLinkCreatedMessage) (*models.ProjectLink, error) { return storedLinkFor(e), nil },
			mutate: func(e *events.ProjectLinkCreatedMessage) { e.FolderUID = "folder-x" },
		},
	}
	for _, tt := range forgedLinkTests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &domainmocks.MockProjectRepository{}
			mockMsg := &domainmocks.MockMessageBuilder{}
			mockLinks := &domainmocks.MockLinkRepository{}

			genuine := events.ProjectLinkCreatedMessage{ProjectUID: "proj-1", LinkUID: "link-1", Name: "Spec Link", URL: "https://specs.example.com", CreatedBy: "uploader"}
			link, err := tt.stored(genuine)
			mockLinks.On("GetLink", mock.Anything, "proj-1", "link-1").Return(link, uint64(1), err)

			forged := genuine
			tt.mutate(&forged)

			svc := &ProjectsService{
				ProjectRepository: mockRepo,
				LinkRepository:    mockLinks,
				FolderRepository:  &domainmocks.MockFolderRepository{},
				Sender:            mockMsg,
				Config:            ServiceConfig{EmailsEnabled: true, LFXSelfServeBaseURL: "https://app.dev.lfx.dev"},
			}

			msg := domainmocks.NewMockMessage(marshalEvent(t, forged), "")
			assert.NoError(t, svc.HandleProjectLinkCreated(context.Background(), msg))

			// No project/roster lookup and no email: the event never reaches the fan-out.
			mockRepo.AssertNotCalled(t, "GetProjectBase", mock.Anything, mock.Anything)
			mockRepo.AssertNotCalled(t, "GetProjectSettings", mock.Anything, mock.Anything)
			mockMsg.AssertNotCalled(t, "SendEmailRequest", mock.Anything, mock.Anything)
		})
	}
}

func TestCollectDocumentRecipients(t *testing.T) {
	writer := models.UserInfo{Username: "alice", Email: "alice@example.com", Name: "Alice"}
	auditor := models.UserInfo{Username: "bob", Email: "bob@example.com", Name: "Bob"}
	noLFID := models.UserInfo{Email: "nolfid@example.com"}
	noEmail := models.UserInfo{Username: "noemail"}

	t.Run("nil settings returns nil", func(t *testing.T) {
		assert.Nil(t, collectDocumentRecipients(nil, ""))
	})

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
