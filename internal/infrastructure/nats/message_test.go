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
		{
			name:   "complex nested json with mapstructure",
			action: models.ActionCreated,
			data:   []byte(`{"uid": "test-project", "metadata": {"tags": ["tag1", "tag2"], "count": 42}, "active": true}`),
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.ProjectMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					// Verify the data is properly converted to map[string]any
					dataMap, ok := msg.Data.(map[string]interface{})
					if !ok {
						return false
					}
					// Check nested structure
					metadata, ok := dataMap["metadata"].(map[string]interface{})
					if !ok {
						return false
					}
					tags, ok := metadata["tags"].([]interface{})
					if !ok || len(tags) != 2 {
						return false
					}
					return dataMap["uid"] == "test-project" && dataMap["active"] == true
				})).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:   "invalid json data",
			action: models.ActionCreated,
			data:   []byte(`{invalid json`),
			setupMocks: func(mockConn *MockNATSConn) {
				// Should not reach publish due to JSON error
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
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, []byte(`{"uid": "test-project"}`)).Return(nil)
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
				mockConn.On("Publish", constants.UpdateAccessProjectSettingsSubject, []byte(`{"uid": "test-project"}`)).Return(nil)
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
				mockConn.On("Publish", constants.DeleteAllAccessSubject, []byte(`{"uid": "test-project"}`)).Return(nil)
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
				mockConn.On("Publish", constants.DeleteAllAccessProjectSettingsSubject, []byte(`{"uid": "test-project"}`)).Return(nil)
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

			return msg.Headers[constants.AuthorizationHeader] == expectedAuth &&
				msg.Headers[constants.XOnBehalfOfHeader] == expectedPrincipal
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
		name           string
		action         models.MessageAction
		data           []byte
		expectedResult interface{}
		wantErr        bool
	}{
		{
			name:   "message with simple object",
			action: models.ActionCreated,
			data:   []byte(`{"message": "simple string"}`),
			expectedResult: map[string]interface{}{
				"message": "simple string",
			},
		},
		{
			name:   "message with object data",
			action: models.ActionUpdated,
			data:   []byte(`{"uid": "test", "name": "Test Project"}`),
			expectedResult: map[string]interface{}{
				"uid":  "test",
				"name": "Test Project",
			},
		},
		{
			name:   "message with array in object",
			action: models.ActionDeleted,
			data:   []byte(`{"items": ["item1", "item2"]}`),
			expectedResult: map[string]interface{}{
				"items": []interface{}{"item1", "item2"},
			},
		},
		{
			name:   "message with nested object",
			action: models.ActionUpdated,
			data:   []byte(`{"project": {"name": "test", "tags": ["go", "nats"]}, "count": 5}`),
			expectedResult: map[string]interface{}{
				"project": map[string]interface{}{
					"name": "test",
					"tags": []interface{}{"go", "nats"},
				},
				"count": float64(5), // JSON numbers are float64
			},
		},
		{
			name:    "message with invalid json",
			action:  models.ActionCreated,
			data:    []byte(`{invalid`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}

			if !tt.wantErr {
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

					// Compare the actual data with expected
					return assert.ObjectsAreEqual(tt.expectedResult, msg.Data)
				})).Return(nil)
			}

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendIndexProject(context.Background(), tt.action, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

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
		assert.Equal(t, "Bearer integration-token", publishedMessage.Headers[constants.AuthorizationHeader])
		assert.Equal(t, "integration-user", publishedMessage.Headers[constants.XOnBehalfOfHeader])

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

func TestMessageBuilder_MapstructureDecoding(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		expectedData map[string]interface{}
	}{
		{
			name: "simple struct with json tags",
			data: []byte(`{"uid": "123", "project_name": "Test"}`),
			expectedData: map[string]interface{}{
				"uid":          "123",
				"project_name": "Test",
			},
		},
		{
			name: "nested struct with arrays",
			data: []byte(`{"metadata": {"tags": ["go", "nats"], "version": 1}, "active": true}`),
			expectedData: map[string]interface{}{
				"metadata": map[string]interface{}{
					"tags":    []interface{}{"go", "nats"},
					"version": float64(1),
				},
				"active": true,
			},
		},
		{
			name: "mixed types",
			data: []byte(`{"string": "text", "number": 42, "bool": false, "null": null, "array": [1, 2, 3]}`),
			expectedData: map[string]interface{}{
				"string": "text",
				"number": float64(42),
				"bool":   false,
				"null":   nil,
				"array":  []interface{}{float64(1), float64(2), float64(3)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}

			mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
				var msg models.ProjectMessage
				err := json.Unmarshal(data, &msg)
				if err != nil {
					return false
				}

				// Verify data is properly decoded
				dataMap, ok := msg.Data.(map[string]interface{})
				if !ok {
					return false
				}

				// Deep comparison of maps
				return assert.ObjectsAreEqual(tt.expectedData, dataMap)
			})).Return(nil)

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendIndexProject(context.Background(), models.ActionCreated, tt.data)
			assert.NoError(t, err)

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_ErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "invalid json",
			data:    []byte(`{"unclosed`),
			wantErr: true,
			errMsg:  "unexpected end of JSON input",
		},
		{
			name:    "empty data",
			data:    []byte(``),
			wantErr: true,
			errMsg:  "unexpected end of JSON input",
		},
		{
			name:    "null data",
			data:    nil,
			wantErr: true,
			errMsg:  "unexpected end of JSON input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}

			builder := &MessageBuilder{
				NatsConn: mockConn,
			}

			err := builder.SendIndexProject(context.Background(), models.ActionCreated, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}
