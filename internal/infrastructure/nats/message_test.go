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
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// backgroundCtx is a reusable function that returns context.Background()
// to satisfy the gocritic unlambda linter rule
var backgroundCtx = context.Background

func TestMessageBuilder_PublishIndexerMessage(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		message     interface{}
		setupMocks  func(*MockNATSConn)
		setupCtx    func() context.Context
		wantErr     bool
		expectedErr error
	}{
		{
			name:    "successful send project indexer message",
			subject: constants.IndexProjectSubject,
			message: models.ProjectIndexerMessage{
				Action: models.ActionCreated,
				Data:   models.ProjectBase{UID: "test-project", Name: "test", Slug: "test"},
				Tags:   []string{"test-project", "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.IndexerMessageEnvelope
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
			name:    "successful send project settings indexer message",
			subject: constants.IndexProjectSettingsSubject,
			message: models.ProjectSettingsIndexerMessage{
				Action: models.ActionUpdated,
				Data:   models.ProjectSettings{UID: "test-settings", MissionStatement: "test mission"},
				Tags:   []string{"test-settings", "test mission"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSettingsSubject, mock.AnythingOfType("[]uint8")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful send delete message",
			subject: constants.IndexProjectSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.AnythingOfType("[]uint8")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "unsupported message type",
			subject: constants.IndexProjectSubject,
			message: 123, // Invalid type
			setupMocks: func(mockConn *MockNATSConn) {
				// No publish expected
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
		{
			name:    "nats publish error",
			subject: constants.IndexProjectSubject,
			message: models.ProjectIndexerMessage{
				Action: models.ActionCreated,
				Data:   models.ProjectBase{UID: "test"},
				Tags:   []string{"test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.AnythingOfType("[]uint8")).Return(errors.New("nats error"))
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := mb.SendIndexerMessage(ctx, tt.subject, tt.message, false)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_PublishIndexerMessage_Sync(t *testing.T) {
	tests := []struct {
		name        string
		subject     string
		message     interface{}
		setupMocks  func(*MockNATSConn)
		setupCtx    func() context.Context
		wantErr     bool
		expectedErr error
	}{
		{
			name:    "successful sync send project indexer message",
			subject: constants.IndexProjectSubject,
			message: models.ProjectIndexerMessage{
				Action: models.ActionCreated,
				Data:   models.ProjectBase{UID: "test-project", Name: "test", Slug: "test"},
				Tags:   []string{"test-project", "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg models.IndexerMessageEnvelope
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == models.ActionCreated
				}), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("ack")}, nil)
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
			name:    "successful sync send project settings indexer message",
			subject: constants.IndexProjectSettingsSubject,
			message: models.ProjectSettingsIndexerMessage{
				Action: models.ActionUpdated,
				Data:   models.ProjectSettings{UID: "test-settings", MissionStatement: "test mission"},
				Tags:   []string{"test-settings", "test mission"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.IndexProjectSettingsSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("ack")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful sync send delete message",
			subject: constants.IndexProjectSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.IndexProjectSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("ack")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "nats request error - sync mode",
			subject: constants.IndexProjectSubject,
			message: models.ProjectIndexerMessage{
				Action: models.ActionCreated,
				Data:   models.ProjectBase{UID: "test"},
				Tags:   []string{"test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.IndexProjectSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(nil, errors.New("nats request timeout"))
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := mb.SendIndexerMessage(ctx, tt.subject, tt.message, true)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_PublishAccessMessage(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		message    interface{}
		setupMocks func(*MockNATSConn)
		setupCtx   func() context.Context
		wantErr    bool
	}{
		{
			name:    "successful send access message",
			subject: constants.UpdateAccessProjectSubject,
			message: models.ProjectAccessMessage{
				Data: models.ProjectAccessData{
					UID:       "test-uid",
					Public:    true,
					ParentUID: "parent-uid",
					Writers:   []string{"user1"},
					Auditors:  []string{"user2"},
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, mock.AnythingOfType("[]uint8")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "unsupported message type",
			subject: constants.UpdateAccessProjectSubject,
			message: 123, // Invalid type - int is not supported
			setupMocks: func(mockConn *MockNATSConn) {
				// No publish expected
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
		{
			name:    "successful send delete access message",
			subject: constants.DeleteAllAccessSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.DeleteAllAccessSubject, []byte("test-uid-to-delete")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "nats publish error",
			subject: constants.UpdateAccessProjectSubject,
			message: models.ProjectAccessMessage{
				Data: models.ProjectAccessData{UID: "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.UpdateAccessProjectSubject, mock.AnythingOfType("[]uint8")).Return(errors.New("nats error"))
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := mb.SendAccessMessage(ctx, tt.subject, tt.message, false)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_PublishAccessMessage_Sync(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		message    interface{}
		setupMocks func(*MockNATSConn)
		setupCtx   func() context.Context
		wantErr    bool
	}{
		{
			name:    "successful sync send access message",
			subject: constants.UpdateAccessProjectSubject,
			message: models.ProjectAccessMessage{
				Data: models.ProjectAccessData{
					UID:       "test-uid",
					Public:    true,
					ParentUID: "parent-uid",
					Writers:   []string{"user1"},
					Auditors:  []string{"user2"},
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.UpdateAccessProjectSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("ack")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful sync send delete access message",
			subject: constants.DeleteAllAccessSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.DeleteAllAccessSubject, []byte("test-uid-to-delete"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("ack")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "nats request error - sync mode",
			subject: constants.UpdateAccessProjectSubject,
			message: models.ProjectAccessMessage{
				Data: models.ProjectAccessData{UID: "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.UpdateAccessProjectSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(nil, errors.New("nats request timeout"))
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := mb.SendAccessMessage(ctx, tt.subject, tt.message, true)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}
