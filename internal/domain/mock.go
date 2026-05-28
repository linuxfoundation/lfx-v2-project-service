// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/stretchr/testify/mock"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// MockProjectRepository implements ProjectRepository for testing
type MockProjectRepository struct {
	mock.Mock
}

func (m *MockProjectRepository) GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ProjectBase), args.Error(1)
}

func (m *MockProjectRepository) GetProjectBaseWithRevision(ctx context.Context, projectUID string) (*models.ProjectBase, uint64, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*models.ProjectBase), args.Get(1).(uint64), args.Error(2)
}

func (m *MockProjectRepository) UpdateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error {
	args := m.Called(ctx, projectBase, revision)
	return args.Error(0)
}

func (m *MockProjectRepository) ProjectExists(ctx context.Context, projectUID string) (bool, error) {
	args := m.Called(ctx, projectUID)
	return args.Bool(0), args.Error(1)
}

func (m *MockProjectRepository) GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ProjectSettings), args.Error(1)
}

func (m *MockProjectRepository) GetProjectSettingsWithRevision(ctx context.Context, projectUID string) (*models.ProjectSettings, uint64, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*models.ProjectSettings), args.Get(1).(uint64), args.Error(2)
}

func (m *MockProjectRepository) UpdateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error {
	args := m.Called(ctx, projectSettings, revision)
	return args.Error(0)
}

func (m *MockProjectRepository) GetProjectUIDFromSlug(ctx context.Context, projectSlug string) (string, error) {
	args := m.Called(ctx, projectSlug)
	return args.String(0), args.Error(1)
}

func (m *MockProjectRepository) ProjectSlugExists(ctx context.Context, projectSlug string) (bool, error) {
	args := m.Called(ctx, projectSlug)
	return args.Bool(0), args.Error(1)
}

func (m *MockProjectRepository) ListAllProjects(ctx context.Context) ([]*models.ProjectBase, []*models.ProjectSettings, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).([]*models.ProjectBase), args.Get(1).([]*models.ProjectSettings), args.Error(2)
}

func (m *MockProjectRepository) ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ProjectBase), args.Error(1)
}

func (m *MockProjectRepository) ListAllProjectsSettings(ctx context.Context) ([]*models.ProjectSettings, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ProjectSettings), args.Error(1)
}

func (m *MockProjectRepository) CreateProject(ctx context.Context, projectBase *models.ProjectBase, projectSettings *models.ProjectSettings) error {
	args := m.Called(ctx, projectBase, projectSettings)
	return args.Error(0)
}

func (m *MockProjectRepository) CreateInviteMapping(ctx context.Context, inviteUID, projectUID string) error {
	args := m.Called(ctx, inviteUID, projectUID)
	return args.Error(0)
}

func (m *MockProjectRepository) GetProjectUIDByInviteUID(ctx context.Context, inviteUID string) (string, error) {
	args := m.Called(ctx, inviteUID)
	return args.String(0), args.Error(1)
}

func (m *MockProjectRepository) DeleteInviteMapping(ctx context.Context, inviteUID string) error {
	args := m.Called(ctx, inviteUID)
	return args.Error(0)
}

func (m *MockProjectRepository) DeleteProject(ctx context.Context, projectUID string, revision uint64) error {
	args := m.Called(ctx, projectUID, revision)
	return args.Error(0)
}

// MockDocumentRepository implements DocumentRepository for testing.
type MockDocumentRepository struct {
	mock.Mock
}

func (m *MockDocumentRepository) GetDocumentMetadata(ctx context.Context, projectUID, documentUID string) (*models.ProjectDocument, uint64, error) {
	args := m.Called(ctx, projectUID, documentUID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*models.ProjectDocument), args.Get(1).(uint64), args.Error(2)
}

func (m *MockDocumentRepository) GetDocumentFile(ctx context.Context, documentUID string) ([]byte, error) {
	args := m.Called(ctx, documentUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockDocumentRepository) CreateDocumentMetadata(ctx context.Context, doc *models.ProjectDocument) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockDocumentRepository) PutDocumentFile(ctx context.Context, documentUID string, fileData []byte) error {
	args := m.Called(ctx, documentUID, fileData)
	return args.Error(0)
}

func (m *MockDocumentRepository) DeleteDocumentMetadata(ctx context.Context, projectUID, documentUID string, revision uint64) error {
	args := m.Called(ctx, projectUID, documentUID, revision)
	return args.Error(0)
}

func (m *MockDocumentRepository) DeleteDocumentFile(ctx context.Context, documentUID string) error {
	args := m.Called(ctx, documentUID)
	return args.Error(0)
}

func (m *MockDocumentRepository) ListDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ProjectDocument), args.Error(1)
}

func (m *MockDocumentRepository) UniqueDocumentName(ctx context.Context, doc *models.ProjectDocument) (string, error) {
	args := m.Called(ctx, doc)
	return args.String(0), args.Error(1)
}

func (m *MockDocumentRepository) DeleteUniqueDocumentName(ctx context.Context, uniqueKey string) error {
	args := m.Called(ctx, uniqueKey)
	return args.Error(0)
}

// MockLinkRepository implements LinkRepository for testing.
type MockLinkRepository struct {
	mock.Mock
}

func (m *MockLinkRepository) GetLink(ctx context.Context, projectUID, linkUID string) (*models.ProjectLink, uint64, error) {
	args := m.Called(ctx, projectUID, linkUID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*models.ProjectLink), args.Get(1).(uint64), args.Error(2)
}

func (m *MockLinkRepository) ListLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	args := m.Called(ctx, projectUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ProjectLink), args.Error(1)
}

func (m *MockLinkRepository) CreateLink(ctx context.Context, link *models.ProjectLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *MockLinkRepository) DeleteLink(ctx context.Context, projectUID, linkUID string, revision uint64) error {
	args := m.Called(ctx, projectUID, linkUID, revision)
	return args.Error(0)
}

// MockFolderRepository implements FolderRepository for testing.
type MockFolderRepository struct {
	mock.Mock
}

func (m *MockFolderRepository) GetFolder(ctx context.Context, projectUID, folderUID string) (*models.ProjectFolder, uint64, error) {
	args := m.Called(ctx, projectUID, folderUID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(uint64), args.Error(2)
	}
	return args.Get(0).(*models.ProjectFolder), args.Get(1).(uint64), args.Error(2)
}

func (m *MockFolderRepository) CreateFolder(ctx context.Context, folder *models.ProjectFolder) error {
	args := m.Called(ctx, folder)
	return args.Error(0)
}

func (m *MockFolderRepository) DeleteFolder(ctx context.Context, projectUID, folderUID string, revision uint64) error {
	args := m.Called(ctx, projectUID, folderUID, revision)
	return args.Error(0)
}

func (m *MockFolderRepository) UniqueFolderName(ctx context.Context, folder *models.ProjectFolder) (string, error) {
	args := m.Called(ctx, folder)
	return args.String(0), args.Error(1)
}

func (m *MockFolderRepository) DeleteUniqueFolderName(ctx context.Context, uniqueKey string) error {
	args := m.Called(ctx, uniqueKey)
	return args.Error(0)
}

// MockMessageBuilder implements MessageBuilder for testing
type MockMessageBuilder struct {
	mock.Mock
}

func (m *MockMessageBuilder) SendIndexerMessage(ctx context.Context, subject string, message any, sync bool) error {
	args := m.Called(ctx, subject, message, sync)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendAccessMessage(ctx context.Context, subject string, message any, sync bool) error {
	args := m.Called(ctx, subject, message, sync)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendProjectEventMessage(ctx context.Context, subject string, message any) error {
	args := m.Called(ctx, subject, message)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendEmailRequest(ctx context.Context, req emailapi.SendEmailRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendInviteRequest(ctx context.Context, req inviteapi.SendInviteRequest) (InviteResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return InviteResult{}, args.Error(1)
	}
	return args.Get(0).(InviteResult), args.Error(1)
}

// MockUserReader implements UserReader for testing.
type MockUserReader struct {
	mock.Mock
}

func (m *MockUserReader) UserMetadataByPrincipal(ctx context.Context, principal string) (*UserMetadata, error) {
	args := m.Called(ctx, principal)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserMetadata), args.Error(1)
}

func (m *MockUserReader) SubByEmail(ctx context.Context, email string) (string, error) {
	args := m.Called(ctx, email)
	return args.String(0), args.Error(1)
}

// MockMessage implements Message for testing
type MockMessage struct {
	mock.Mock
	data    []byte
	subject string
}

func (m *MockMessage) Subject() string {
	return m.subject
}

func (m *MockMessage) Data() []byte {
	return m.data
}

func (m *MockMessage) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}

// NewMockMessage creates a mock message for testing
func NewMockMessage(data []byte, subject string) *MockMessage {
	return &MockMessage{
		data:    data,
		subject: subject,
	}
}
