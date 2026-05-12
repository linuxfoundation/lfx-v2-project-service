// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionCreated,
				Data:   models.ProjectBase{UID: "test-project", Name: "test", Slug: "test"},
				Tags:   []string{"test-project", "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg indexerTypes.IndexerMessageEnvelope
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == indexerConstants.ActionCreated
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionUpdated,
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionCreated,
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionCreated,
				Data:   models.ProjectBase{UID: "test-project", Name: "test", Slug: "test"},
				Tags:   []string{"test-project", "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", constants.IndexProjectSubject, mock.MatchedBy(func(data []byte) bool {
					var msg indexerTypes.IndexerMessageEnvelope
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.Action == indexerConstants.ActionCreated
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionUpdated,
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
			message: indexerTypes.IndexerMessageEnvelope{
				Action: indexerConstants.ActionCreated,
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
			name:    "successful send update access message",
			subject: fgaconstants.GenericUpdateAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "update_access",
				Data: fgatypes.GenericAccessData{
					UID:    "test-uid",
					Public: true,
					Relations: map[string][]string{
						"writer":  {"user1"},
						"auditor": {"user2"},
					},
					References: map[string][]string{
						"parent": {"project:parent-uid"},
					},
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("[]uint8")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful send delete access message",
			subject: fgaconstants.GenericDeleteAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "delete_access",
				Data: fgatypes.GenericDeleteData{
					UID: "test-uid-to-delete",
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", fgaconstants.GenericDeleteAccessSubject, mock.AnythingOfType("[]uint8")).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "unsupported message type",
			subject: fgaconstants.GenericUpdateAccessSubject,
			message: 123, // Invalid type - int is not supported
			setupMocks: func(mockConn *MockNATSConn) {
				// No publish expected
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
		{
			name:    "nats publish error",
			subject: fgaconstants.GenericUpdateAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "update_access",
				Data:       fgatypes.GenericAccessData{UID: "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("[]uint8")).Return(errors.New("nats error"))
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
			name:    "successful sync send update access message",
			subject: fgaconstants.GenericUpdateAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "update_access",
				Data: fgatypes.GenericAccessData{
					UID:    "test-uid",
					Public: true,
					Relations: map[string][]string{
						"writer":  {"user1"},
						"auditor": {"user2"},
					},
					References: map[string][]string{
						"parent": {"project:parent-uid"},
					},
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("OK")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful sync send delete access message",
			subject: fgaconstants.GenericDeleteAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "delete_access",
				Data: fgatypes.GenericDeleteData{
					UID: "test-uid-to-delete",
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", fgaconstants.GenericDeleteAccessSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(&nats.Msg{Data: []byte("OK")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "nats request error - sync mode",
			subject: fgaconstants.GenericUpdateAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "update_access",
				Data:       fgatypes.GenericAccessData{UID: "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Request", fgaconstants.GenericUpdateAccessSubject, mock.AnythingOfType("[]uint8"), defaultRequestTimeout).Return(nil, errors.New("nats request timeout"))
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

func TestMessageBuilder_SendEmailRequest(t *testing.T) {
	req := emailapi.SendEmailRequest{
		To:      "alice@example.com",
		Subject: "You've been added",
		HTML:    "<p>Hi Alice</p>",
		Text:    "Hi Alice",
	}

	tests := []struct {
		name      string
		mockSetup func(*MockNATSConn)
		wantErr   bool
	}{
		{
			name: "success — empty reply body",
			mockSetup: func(m *MockNATSConn) {
				m.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == emailapi.SendEmailSubject
				})).Return(&nats.Msg{Data: nil}, nil)
			},
			wantErr: false,
		},
		{
			name: "success — non-error reply body",
			mockSetup: func(m *MockNATSConn) {
				m.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == emailapi.SendEmailSubject
				})).Return(&nats.Msg{Data: []byte(`{}`)}, nil)
			},
			wantErr: false,
		},
		{
			name: "NATS transport error",
			mockSetup: func(m *MockNATSConn) {
				m.On("RequestMsgWithContext", mock.Anything, mock.Anything).
					Return(nil, errors.New("connection closed"))
			},
			wantErr: true,
		},
		{
			name: "email service returns error response",
			mockSetup: func(m *MockNATSConn) {
				errBody, _ := json.Marshal(emailapi.SendEmailErrorResponse{Error: "smtp refused"})
				m.On("RequestMsgWithContext", mock.Anything, mock.Anything).
					Return(&nats.Msg{Data: errBody}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.mockSetup(mockConn)

			mb := &MessageBuilder{NatsConn: mockConn}
			err := mb.SendEmailRequest(context.Background(), req)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_SendProjectEventMessage(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		message    interface{}
		setupMocks func(*MockNATSConn)
		wantErr    bool
	}{
		{
			name:    "successful send project settings updated message",
			subject: constants.ProjectSettingsUpdatedSubject,
			message: events.ProjectSettingsUpdatedMessage{
				ProjectUID: "test-project-uid",
				OldSettings: events.ProjectSettings{
					UID:              "test-project-uid",
					MissionStatement: "old mission",
				},
				NewSettings: events.ProjectSettings{
					UID:              "test-project-uid",
					MissionStatement: "new mission",
				},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.ProjectSettingsUpdatedSubject, mock.MatchedBy(func(data []byte) bool {
					var msg events.ProjectSettingsUpdatedMessage
					err := json.Unmarshal(data, &msg)
					if err != nil {
						return false
					}
					return msg.ProjectUID == "test-project-uid" &&
						msg.OldSettings.MissionStatement == "old mission" &&
						msg.NewSettings.MissionStatement == "new mission"
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name:    "nats publish error",
			subject: constants.ProjectSettingsUpdatedSubject,
			message: events.ProjectSettingsUpdatedMessage{
				ProjectUID:  "test-project-uid",
				OldSettings: events.ProjectSettings{UID: "test"},
				NewSettings: events.ProjectSettings{UID: "test"},
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("Publish", constants.ProjectSettingsUpdatedSubject, mock.AnythingOfType("[]uint8")).Return(errors.New("nats error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := context.Background()
			err := mb.SendProjectEventMessage(ctx, tt.subject, tt.message)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)
		})
	}
}
