// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// backgroundCtx is a reusable function that returns context.Background()
// to satisfy the gocritic unlambda linter rule
var backgroundCtx = context.Background

func TestMessageBuilder_SendIndexProject(t *testing.T) {
	tests := []struct {
		name        string
		action      models.MessageAction
		data        models.ProjectBase
		setupMocks  func(*MockNATSConn)
		setupCtx    func() context.Context
		wantErr     bool
		expectedErr error
	}{
		{
			name:   "successful send index project message",
			action: models.ActionCreated,
			data:   models.ProjectBase{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == models.ActionCreated
				})).Return(nil)
			},
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = context.WithValue(ctx, constants.AuthorizationContextID, "Bearer token123")
				ctx = context.WithValue(ctx, constants.PrincipalContextID, "user123")
				return ctx
			},
			wantErr: false,
		},
		{
			name:   "send index project message without headers",
			action: models.ActionUpdated,
			data:   models.ProjectBase{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == models.ActionUpdated && len(msg.Headers) == 0
				})).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:   "verify tags are set correctly",
			action: models.ActionCreated,
			data:   models.ProjectBase{UID: "test-uid", Name: "Test Project", Slug: "test-project", Description: "Test Description"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					expectedTags := []string{"test-uid", "Test Project", "test-project", "Test Description"}
					return len(msg.Tags) == len(expectedTags) &&
						msg.Tags[0] == expectedTags[0] &&
						msg.Tags[1] == expectedTags[1] &&
						msg.Tags[2] == expectedTags[2] &&
						msg.Tags[3] == expectedTags[3]
				})).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:   "nats publish error",
			action: models.ActionUpdated,
			data:   models.ProjectBase{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.Anything).Return(errors.New("nats connection error"))
			},
			setupCtx:    backgroundCtx,
			wantErr:     true,
			expectedErr: errors.New("nats connection error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := builder.SendIndexProject(ctx, tt.action, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.Contains(t, err.Error(), tt.expectedErr.Error())
				}
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendIndexProjectSettings(t *testing.T) {
	tests := []struct {
		name       string
		action     models.MessageAction
		data       models.ProjectSettings
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name:   "successful send index project settings message",
			action: models.ActionCreated,
			data:   models.ProjectSettings{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionCreated
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name:   "verify tags are set correctly",
			action: models.ActionCreated,
			data:   models.ProjectSettings{UID: "test-uid", MissionStatement: "Test Mission"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					expectedTags := []string{"test-uid", "Test Mission"}
					return len(msg.Tags) == len(expectedTags) &&
						msg.Tags[0] == expectedTags[0] &&
						msg.Tags[1] == expectedTags[1]
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name:   "nats publish error",
			action: models.ActionUpdated,
			data:   models.ProjectSettings{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.Anything).Return(errors.New("publish failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendIndexProjectSettings(context.Background(), tt.action, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendDeleteIndexProject(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		setupMocks func(*MockNATSConn)
		setupCtx   func() context.Context
		wantErr    bool
	}{
		{
			name: "successful send delete index project message",
			data: "test-project-uid",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == models.ActionDeleted && msg.Tags == nil
				})).Return(nil)
			},
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = context.WithValue(ctx, constants.AuthorizationContextID, "Bearer token123")
				return ctx
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: "test-project-uid",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.Anything).Return(errors.New("publish failed"))
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := builder.SendDeleteIndexProject(ctx, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendDeleteIndexProjectSettings(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send delete index project settings message",
			data: "test-project-uid",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectIndexerMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == models.ActionDeleted && msg.Tags == nil
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: "test-project-uid",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.Anything).Return(errors.New("publish failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendDeleteIndexProjectSettings(context.Background(), tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendUpdateAccessProject(t *testing.T) {
	tests := []struct {
		name       string
		data       models.ProjectAccessMessage
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send update access project message",
			data: models.ProjectAccessMessage{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, mock.Anything).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: models.ProjectAccessMessage{UID: "test-project"},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, mock.Anything).Return(errors.New("publish failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendUpdateAccessProject(context.Background(), tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendDeleteAllAccessProject(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send delete all access project message",
			data: "test-project",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessSubject, []byte("test-project")).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: "test-project",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessSubject, mock.Anything).Return(errors.New("publish failed"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendDeleteAllAccessProject(context.Background(), tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_ContextHandling(t *testing.T) {
	t.Run("extracts authorization and principal from context", func(t *testing.T) {
		mockConn := &MockNATSConn{}

		expectedAuth := "Bearer token123"
		expectedPrincipal := "user456"

		mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
			var msg models.ProjectIndexerMessage
			err := json.Unmarshal(data, &msg)
			if err != nil {
				return false
			}

			return msg.Headers[constants.AuthorizationHeader] == expectedAuth &&
				msg.Headers[constants.XOnBehalfOfHeader] == expectedPrincipal
		})).Return(nil)

		builder := &MessageBuilder{
			NatsConn: mockConn,
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, constants.AuthorizationContextID, expectedAuth)
		ctx = context.WithValue(ctx, constants.PrincipalContextID, expectedPrincipal)

		err := builder.SendIndexProject(ctx, models.ActionCreated, models.ProjectBase{UID: "test-project"})
		assert.NoError(t, err)

		mockConn.AssertExpectations(t)
	})

	t.Run("handles context without headers", func(t *testing.T) {
		mockConn := &MockNATSConn{}

		mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
			var msg models.ProjectIndexerMessage
			err := json.Unmarshal(data, &msg)
			if err != nil {
				return false
			}

			// Should have empty headers map
			return len(msg.Headers) == 0
		})).Return(nil)

		builder := &MessageBuilder{
			NatsConn: mockConn,
		}

		err := builder.SendIndexProject(context.Background(), models.ActionCreated, models.ProjectBase{UID: "test-project"})
		assert.NoError(t, err)

		mockConn.AssertExpectations(t)
	})
}
