// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// backgroundCtx is a reusable function that returns context.Background()
// to satisfy the gocritic unlambda linter rule
var backgroundCtx = context.Background

func TestMessageBuilder_SendIndexProject(t *testing.T) {
	tests := []struct {
		name        string
		action      models.MessageAction
		data        []byte
		setupMocks  func(*MockNATSConn)
		setupCtx    func() context.Context
		wantErr     bool
		expectedErr error
	}{
		{
			name:   "successful send index project message",
			action: models.ActionCreated,
			data:   []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
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
			data:   []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
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
			name:   "nats publish error",
			action: models.ActionDeleted,
			data:   []byte(`{"uid": "test-project"}`),
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
		data       []byte
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name:   "successful send index project settings message",
			action: models.ActionCreated,
			data:   []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionCreated
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name:   "nats publish error",
			action: models.ActionUpdated,
			data:   []byte(`{"uid": "test-project"}`),
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

func TestMessageBuilder_SendUpdateAccessProject(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send update access project message",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionUpdated
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: []byte(`{"uid": "test-project"}`),
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

func TestMessageBuilder_SendUpdateAccessProjectSettings(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send update access project settings message",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionUpdated
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSettingsSubject, mock.Anything).Return(errors.New("publish failed"))
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

			err := builder.SendUpdateAccessProjectSettings(context.Background(), tt.data)

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
		data       []byte
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send delete all access project message",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionDeleted
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: []byte(`{"uid": "test-project"}`),
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

func TestMessageBuilder_SendDeleteAllAccessProjectSettings(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name: "successful send delete all access project settings message",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessProjectSettingsSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					return err == nil && msg.Action == models.ActionDeleted
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "nats publish error",
			data: []byte(`{"uid": "test-project"}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessProjectSettingsSubject, mock.Anything).Return(errors.New("publish failed"))
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

			err := builder.SendDeleteAllAccessProjectSettings(context.Background(), tt.data)

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
			var msg models.ProjectMessage
			err := json.Unmarshal(data, &msg)
			if err != nil {
				return false
			}

			return msg.Headers["authorization"] == expectedAuth &&
				msg.Headers["x-on-behalf-of"] == expectedPrincipal
		})).Return(nil)

		builder := &MessageBuilder{
			NatsConn: mockConn,
		}

		ctx := context.Background()
		ctx = context.WithValue(ctx, constants.AuthorizationContextID, expectedAuth)
		ctx = context.WithValue(ctx, constants.PrincipalContextID, expectedPrincipal)

		err := builder.SendIndexProject(ctx, models.ActionCreated, []byte(`{"test": "data"}`))
		assert.NoError(t, err)

		mockConn.AssertExpectations(t)
	})

	t.Run("handles context without headers", func(t *testing.T) {
		mockConn := &MockNATSConn{}

		mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
			var msg models.ProjectMessage
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

		err := builder.SendIndexProject(context.Background(), models.ActionCreated, []byte(`{"test": "data"}`))
		assert.NoError(t, err)

		mockConn.AssertExpectations(t)
	})
}

func TestMessageBuilder_MessageConstruction(t *testing.T) {
	tests := []struct {
		name         string
		action       models.MessageAction
		data         []byte
		expectedData interface{}
	}{
		{
			name:         "message with string data",
			action:       models.ActionCreated,
			data:         []byte(`"simple string"`),
			expectedData: []byte(`"simple string"`),
		},
		{
			name:         "message with object data",
			action:       models.ActionUpdated,
			data:         []byte(`{"uid": "test", "name": "Test Project"}`),
			expectedData: []byte(`{"uid": "test", "name": "Test Project"}`),
		},
		{
			name:         "message with array data",
			action:       models.ActionDeleted,
			data:         []byte(`["item1", "item2"]`),
			expectedData: []byte(`["item1", "item2"]`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}

			mockConn.On("Publish", mock.Anything, mock.MatchedBy(func(data []byte) bool {
				var msg models.ProjectMessage
				err := json.Unmarshal(data, &msg)
				if err != nil {
					return false
				}

				// Verify action
				if msg.Action != tt.action {
					return false
				}

				// Verify data field matches expected
				msgDataBytes, err := json.Marshal(msg.Data)
				if err != nil {
					return false
				}

				expectedDataStr, ok := tt.expectedData.([]byte)
				if !ok {
					return false
				}

				// For robust comparison, unmarshal both and compare as objects
				var msgDataObj, expectedDataObj any
				if err := json.Unmarshal(msgDataBytes, &msgDataObj); err != nil {
					return false
				}
				if err := json.Unmarshal(expectedDataStr, &expectedDataObj); err != nil {
					return false
				}

				// Use deep comparison instead of string comparison
				return fmt.Sprintf("%v", msgDataObj) == fmt.Sprintf("%v", expectedDataObj)
			})).Return(nil)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendIndexProject(context.Background(), tt.action, tt.data)
			assert.NoError(t, err)

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_Integration(t *testing.T) {
	t.Run("end to end message building and publishing", func(t *testing.T) {
		mockConn := &MockNATSConn{}

		// Set up context with full headers
		ctx := context.Background()
		ctx = context.WithValue(ctx, constants.AuthorizationContextID, "Bearer integration-token")
		ctx = context.WithValue(ctx, constants.PrincipalContextID, "integration-user")

		// Capture the published message for verification
		var publishedMessage models.ProjectMessage
		mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
			err := json.Unmarshal(data, &publishedMessage)
			return err == nil
		})).Return(nil)

		builder := &MessageBuilder{
			NatsConn: mockConn,
		}

		projectData := []byte(`{"uid": "integration-test", "name": "Integration Test Project"}`)
		err := builder.SendIndexProject(ctx, models.ActionCreated, projectData)

		require.NoError(t, err)

		// Verify the message structure
		assert.Equal(t, models.ActionCreated, publishedMessage.Action)
		assert.Equal(t, "Bearer integration-token", publishedMessage.Headers["authorization"])
		assert.Equal(t, "integration-user", publishedMessage.Headers["x-on-behalf-of"])

		// Verify data by marshaling both and comparing
		actualDataBytes, err := json.Marshal(publishedMessage.Data)
		require.NoError(t, err)

		var expectedObj, actualObj any
		require.NoError(t, json.Unmarshal(projectData, &expectedObj))
		require.NoError(t, json.Unmarshal(actualDataBytes, &actualObj))
		assert.Equal(t, expectedObj, actualObj)

		mockConn.AssertExpectations(t)
	})
}
