// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import (
	"context"

	"github.com/stretchr/testify/mock"

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

func (m *MockProjectRepository) DeleteProject(ctx context.Context, projectUID string, revision uint64) error {
	args := m.Called(ctx, projectUID, revision)
	return args.Error(0)
}

// MockMessageBuilder implements MessageBuilder for testing
type MockMessageBuilder struct {
	mock.Mock
}

func (m *MockMessageBuilder) SendIndexProject(ctx context.Context, action models.MessageAction, data models.ProjectBase) error {
	args := m.Called(ctx, action, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendDeleteIndexProject(ctx context.Context, data string) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendIndexProjectSettings(ctx context.Context, action models.MessageAction, data models.ProjectSettings) error {
	args := m.Called(ctx, action, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendDeleteIndexProjectSettings(ctx context.Context, data string) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendUpdateAccessProject(ctx context.Context, data models.ProjectBase) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendUpdateAccessProjectSettings(ctx context.Context, data models.ProjectSettings) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendDeleteAllAccessProject(ctx context.Context, data string) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockMessageBuilder) SendDeleteAllAccessProjectSettings(ctx context.Context, data string) error {
	args := m.Called(ctx, data)
	return args.Error(0)
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
