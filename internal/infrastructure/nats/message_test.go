// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	emailapi "github.com/linuxfoundation/lfx-v2-email-service/pkg/api"
	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					if msg.Subject != constants.IndexProjectSubject {
						return false
					}
					var m indexerTypes.IndexerMessageEnvelope
					err := json.Unmarshal(msg.Data, &m)
					if err != nil {
						return false
					}
					return m.Action == indexerConstants.ActionCreated
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.IndexProjectSettingsSubject
				})).Return(nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful send delete message",
			subject: constants.IndexProjectSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.IndexProjectSubject
				})).Return(nil)
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.IndexProjectSubject
				})).Return(errors.New("nats error"))
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

func setupIndexerReply(reply *nats.Msg, err error) func(*MockNATSConn) {
	return func(mockConn *MockNATSConn) {
		mockConn.On("RequestMsgWithContext", mock.Anything, mock.AnythingOfType("*nats.Msg")).
			Return(reply, err)
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
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					if msg.Subject != constants.IndexProjectSubject {
						return false
					}
					var m indexerTypes.IndexerMessageEnvelope
					err := json.Unmarshal(msg.Data, &m)
					if err != nil {
						return false
					}
					return m.Action == indexerConstants.ActionCreated
				})).Return(&nats.Msg{Data: []byte("OK")}, nil)
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
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.IndexProjectSettingsSubject
				})).Return(&nats.Msg{Data: []byte("OK")}, nil)
			},
			setupCtx: backgroundCtx,
			wantErr:  false,
		},
		{
			name:    "successful sync send delete message",
			subject: constants.IndexProjectSubject,
			message: "test-uid-to-delete",
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.IndexProjectSubject
				})).Return(&nats.Msg{Data: []byte("OK")}, nil)
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
			setupMocks: setupIndexerReply(nil, errors.New("nats request timeout")),
			setupCtx:   backgroundCtx,
			wantErr:    true,
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

func TestMessageBuilder_SyncAcknowledgement(t *testing.T) {
	tests := []struct {
		name       string
		reply      *nats.Msg
		requestErr error
		wantErr    bool
	}{
		{name: "exact OK", reply: &nats.Msg{Data: []byte("OK")}},
		{name: "indexer error", reply: &nats.Msg{Data: []byte("ERROR: indexing failed")}, wantErr: true},
		{name: "empty response", reply: &nats.Msg{}, wantErr: true},
		{name: "unexpected response", reply: &nats.Msg{Data: []byte("ack")}, wantErr: true},
		{name: "case variant", reply: &nats.Msg{Data: []byte("ok")}, wantErr: true},
		{name: "whitespace padded", reply: &nats.Msg{Data: []byte(" OK ")}, wantErr: true},
		{name: "nil response", wantErr: true},
		{name: "transport error", requestErr: errors.New("nats request timeout"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			setupIndexerReply(tt.reply, tt.requestErr)(mockConn)
			mb := &MessageBuilder{NatsConn: mockConn}

			err := mb.sendMessage(context.Background(), constants.IndexProjectSubject, []byte("{}"), true)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			mockConn.AssertExpectations(t)
		})
	}
}

func TestMessageBuilder_IndexerLogsExcludeSensitiveContent(t *testing.T) {
	const (
		payloadSentinel       = "payload-secret-sentinel"
		configSentinel        = "config-secret-sentinel"
		authorizationSentinel = "authorization-secret-sentinel"
		replySentinel         = "reply-secret-sentinel"
	)

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	mockConn := &MockNATSConn{}
	mockConn.On("RequestMsgWithContext", mock.Anything, mock.AnythingOfType("*nats.Msg")).
		Return(&nats.Msg{Data: []byte("ERROR: " + replySentinel)}, nil)

	mb := &MessageBuilder{NatsConn: mockConn}
	ctx := context.WithValue(context.Background(), constants.AuthorizationContextID, "Bearer "+authorizationSentinel)
	message := indexerTypes.IndexerMessageEnvelope{
		Action: indexerConstants.ActionUpdated,
		Data: models.ProjectSettings{
			UID:              "00000000-0000-0000-0000-000000000001",
			MissionStatement: payloadSentinel,
		},
		IndexingConfig: &indexerTypes.IndexingConfig{ObjectID: configSentinel},
	}

	err := mb.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, message, true)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), replySentinel)

	output := logs.String()
	assert.Contains(t, output, "subject="+constants.IndexProjectSettingsSubject)
	assert.Contains(t, output, "action=updated")
	assert.Contains(t, output, "indexer did not acknowledge message")
	assert.NotContains(t, output, payloadSentinel)
	assert.NotContains(t, output, configSentinel)
	assert.NotContains(t, output, authorizationSentinel)
	assert.NotContains(t, output, replySentinel)
	mockConn.AssertExpectations(t)
}

// matchFGAEnvelope builds a mock.MatchedBy predicate asserting that a published
// NATS message carries the given subject and decodes to the expected FGA
// envelope fields (object_type, operation, and the data's uid).
func matchFGAEnvelope(subject, operation, uid string) func(msg *nats.Msg) bool {
	return func(msg *nats.Msg) bool {
		if msg.Subject != subject {
			return false
		}
		var envelope struct {
			ObjectType string `json:"object_type"`
			Operation  string `json:"operation"`
			Data       struct {
				UID string `json:"uid"`
			} `json:"data"`
		}
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			return false
		}
		return envelope.ObjectType == "project" &&
			envelope.Operation == operation &&
			envelope.Data.UID == uid
	}
}

// accessMessageSpanRecorder/accessMessageTracerProvider back the trace
// assertions in TestMessageBuilder_PublishAccessMessage. otel.SetTracerProvider
// only performs a one-time delegate upgrade for already-resolved package-level
// Tracer handles (see tracing.go), so it must be installed at most once for
// the lifetime of the test binary — including across repeated runs of the
// same test function under `go test -count=N`. Trace assertions therefore
// compare span-count deltas against this shared recorder instead of resetting it.
var (
	accessMessageTracerOnce     sync.Once
	accessMessageSpanRecorder   *tracetest.SpanRecorder
	accessMessageTracerProvider *sdktrace.TracerProvider
)

func setupAccessMessageTracing() *tracetest.SpanRecorder {
	accessMessageTracerOnce.Do(func() {
		accessMessageSpanRecorder = tracetest.NewSpanRecorder()
		accessMessageTracerProvider = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(accessMessageSpanRecorder))
		otel.SetTracerProvider(accessMessageTracerProvider)
		otel.SetTextMapPropagator(propagation.TraceContext{})
	})
	return accessMessageSpanRecorder
}

func TestMessageBuilder_PublishAccessMessage(t *testing.T) {
	spanRecorder := setupAccessMessageTracing()

	tests := []struct {
		name             string
		subject          string
		message          fgatypes.GenericFGAMessage
		setupMocks       func(*MockNATSConn)
		setupCtx         func() context.Context
		wantErr          bool
		verifyTrace      bool
		verifyFailureLog bool
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
				mockConn.On("PublishMsg", mock.MatchedBy(
					matchFGAEnvelope(fgaconstants.GenericUpdateAccessSubject, "update_access", "test-uid"),
				)).Return(nil)
			},
			setupCtx:    backgroundCtx,
			wantErr:     false,
			verifyTrace: true,
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
				mockConn.On("PublishMsg", mock.MatchedBy(
					matchFGAEnvelope(fgaconstants.GenericDeleteAccessSubject, "delete_access", "test-uid-to-delete"),
				)).Return(nil)
			},
			setupCtx:    backgroundCtx,
			wantErr:     false,
			verifyTrace: true,
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == fgaconstants.GenericUpdateAccessSubject
				})).Return(errors.New("nats error"))
			},
			setupCtx:         backgroundCtx,
			wantErr:          true,
			verifyFailureLog: true,
		},
		{
			name:    "marshal error",
			subject: fgaconstants.GenericDeleteAccessSubject,
			message: fgatypes.GenericFGAMessage{
				ObjectType: "project",
				Operation:  "delete_access",
				Data:       make(chan int),
			},
			setupMocks: func(mockConn *MockNATSConn) {
				// No publish expected when JSON marshalling fails.
			},
			setupCtx: backgroundCtx,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spansBefore := len(spanRecorder.Ended())

			var logs bytes.Buffer
			if tt.verifyFailureLog {
				previousLogger := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
				t.Cleanup(func() {
					slog.SetDefault(previousLogger)
				})
			}

			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{
				NatsConn: mockConn,
			}

			ctx := tt.setupCtx()
			err := mb.PublishAccessMessage(ctx, tt.subject, tt.message)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockConn.AssertExpectations(t)

			if tt.verifyTrace {
				require.NoError(t, accessMessageTracerProvider.ForceFlush(context.Background()))
				spans := spanRecorder.Ended()
				require.Len(t, spans, spansBefore+1)
				newSpan := spans[len(spans)-1]
				assert.Equal(t, "nats.publish", newSpan.Name())
				assert.Equal(t, oteltrace.SpanKindProducer, newSpan.SpanKind())
			}

			if tt.verifyFailureLog {
				assert.Contains(t, logs.String(), "subject="+tt.subject)
			}
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					if msg.Subject != constants.ProjectSettingsUpdatedSubject {
						return false
					}
					var m events.ProjectSettingsUpdatedMessage
					err := json.Unmarshal(msg.Data, &m)
					if err != nil {
						return false
					}
					return m.ProjectUID == "test-project-uid" &&
						m.OldSettings.MissionStatement == "old mission" &&
						m.NewSettings.MissionStatement == "new mission"
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
				mockConn.On("PublishMsg", mock.MatchedBy(func(msg *nats.Msg) bool {
					return msg.Subject == constants.ProjectSettingsUpdatedSubject
				})).Return(errors.New("nats error"))
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

func TestMessageBuilder_SendInviteRequest(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		req        inviteapi.SendInviteRequest
		setupMocks func(*MockNATSConn)
		wantErr    bool
		wantResult domain.InviteResult
	}{
		{
			name: "successful request — correct subject, payload, and invite result returned",
			req: inviteapi.SendInviteRequest{
				Recipient: &inviteapi.Recipient{
					Email: "user@example.com",
					Name:  "Jane Doe",
				},
				Inviter: &inviteapi.Inviter{
					Name: "Admin",
				},
				Resource: &inviteapi.Resource{
					UID:  "proj-123",
					Name: "Demo Project",
					Type: "project",
				},
				Role:      string(inviteapi.InviteRoleManage),
				ReturnURL: "https://app.lfx.dev/project/overview?project=demo",
			},
			setupMocks: func(mockConn *MockNATSConn) {
				expiresAt := now.Add(7 * 24 * time.Hour)
				replyData, _ := json.Marshal(inviteapi.SendInviteResponse{
					InviteData: &inviteapi.InviteData{
						UID:       "invite-abc-123",
						Email:     "user@example.com",
						ExpiresAt: expiresAt,
					},
				})
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *nats.Msg) bool {
					if msg.Subject != inviteapi.SendInviteSubject {
						return false
					}
					var got inviteapi.SendInviteRequest
					if err := json.Unmarshal(msg.Data, &got); err != nil {
						return false
					}
					return got.Recipient != nil &&
						got.Recipient.Email == "user@example.com" &&
						got.Recipient.Name == "Jane Doe" &&
						got.Inviter != nil &&
						got.Inviter.Name == "Admin" &&
						got.Resource != nil &&
						got.Resource.UID == "proj-123" &&
						got.Resource.Name == "Demo Project" &&
						got.Resource.Type == "project" &&
						got.Role == string(inviteapi.InviteRoleManage) &&
						got.ReturnURL == "https://app.lfx.dev/project/overview?project=demo"
				})).Return(&nats.Msg{Data: replyData}, nil)
			},
			wantErr: false,
			wantResult: domain.InviteResult{
				InviteUID:      "invite-abc-123",
				RecipientEmail: "user@example.com",
				ExpiresAt:      now.Add(7 * 24 * time.Hour),
			},
		},
		{
			name: "invite service returns error in response body — error returned",
			req: inviteapi.SendInviteRequest{
				Recipient: &inviteapi.Recipient{Email: "user@example.com"},
				Resource:  &inviteapi.Resource{UID: "proj-123"},
				Role:      string(inviteapi.InviteRoleView),
			},
			setupMocks: func(mockConn *MockNATSConn) {
				replyData, _ := json.Marshal(inviteapi.SendInviteResponse{Error: "recipient not found"})
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.AnythingOfType("*nats.Msg")).
					Return(&nats.Msg{Data: replyData}, nil)
			},
			wantErr: true,
		},
		{
			name: "NATS request error — error returned",
			req: inviteapi.SendInviteRequest{
				Recipient: &inviteapi.Recipient{Email: "user@example.com"},
				Resource:  &inviteapi.Resource{UID: "proj-123"},
				Role:      string(inviteapi.InviteRoleView),
			},
			setupMocks: func(mockConn *MockNATSConn) {
				mockConn.On("RequestMsgWithContext", mock.Anything, mock.AnythingOfType("*nats.Msg")).
					Return(nil, errors.New("nats timeout"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			tt.setupMocks(mockConn)

			mb := &MessageBuilder{NatsConn: mockConn}
			result, err := mb.SendInviteRequest(context.Background(), tt.req)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantResult.InviteUID, result.InviteUID)
				assert.Equal(t, tt.wantResult.RecipientEmail, result.RecipientEmail)
				// Allow a small time difference for test timing
				assert.WithinDuration(t, tt.wantResult.ExpiresAt, result.ExpiresAt, time.Second)
			}

			mockConn.AssertExpectations(t)
		})
	}
}
