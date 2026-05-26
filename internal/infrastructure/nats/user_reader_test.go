// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// replyMsg builds a minimal NATS reply message for success cases.
// Transport-error cases should pass nil as the reply directly.
func replyMsg(data []byte) *natsgo.Msg { return &natsgo.Msg{Data: data} }

func TestUserReaderNATS_SubByEmail(t *testing.T) {
	tests := []struct {
		name       string
		reply      *natsgo.Msg // nil simulates a transport error
		replyErr   error
		wantUser   string
		wantErr    error
		wantErrStr string
	}{
		{
			name:     "plain-text username returned on success",
			reply:    replyMsg([]byte("alice")),
			wantUser: "alice",
		},
		{
			name:     "trailing newline trimmed from username",
			reply:    replyMsg([]byte("alice\n")),
			wantUser: "alice",
		},
		{
			name:     "leading and trailing whitespace trimmed",
			reply:    replyMsg([]byte("  alice  ")),
			wantUser: "alice",
		},
		{
			name:    "empty body returns ErrUserNotFound",
			reply:   replyMsg([]byte("")),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "whitespace-only body returns ErrUserNotFound",
			reply:   replyMsg([]byte("   \n  ")),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "JSON error envelope returns ErrUserNotFound",
			reply:   replyMsg([]byte(`{"success":false,"error":"user not found"}`)),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "JSON success envelope returns ErrUserNotFound instead of leaking JSON as username",
			reply:   replyMsg([]byte(`{"success":true,"username":"alice"}`)),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:    "malformed JSON object returns ErrUserNotFound instead of leaking raw body as username",
			reply:   replyMsg([]byte(`{"success":"true"}`)),
			wantErr: domain.ErrUserNotFound,
		},
		{
			name:       "transport error is wrapped and returned",
			reply:      nil,
			replyErr:   errors.New("nats: connection closed"),
			wantErrStr: "email_to_sub request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *natsgo.Msg) bool {
				return msg.Subject == constants.AuthEmailToSubSubject
			})).Return(tt.reply, tt.replyErr)

			reader := &UserReaderNATS{NatsConn: mockConn}
			got, err := reader.SubByEmail(context.Background(), "test@example.com")

			switch {
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
			case tt.wantErrStr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				assert.Empty(t, got)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.wantUser, got)
			}
			mockConn.AssertExpectations(t)
		})
	}
}

func TestUserReaderNATS_UserMetadataByPrincipal(t *testing.T) {
	marshalSuccess := func(data map[string]interface{}) []byte {
		b, err := json.Marshal(map[string]interface{}{"success": true, "data": data})
		require.NoError(t, err)
		return b
	}

	tests := []struct {
		name       string
		reply      *natsgo.Msg // nil simulates a transport error
		replyErr   error
		wantMeta   *domain.UserMetadata
		wantErrStr string
	}{
		{
			name: "all fields populated",
			reply: replyMsg(marshalSuccess(map[string]interface{}{
				"name":           "Alice Example",
				"given_name":     "Alice",
				"family_name":    "Example",
				"picture":        "https://example.com/alice.png",
				"zoneinfo":       "America/New_York",
				"job_title":      "Engineer",
				"organization":   "LF",
				"country":        "US",
				"state_province": "CA",
				"city":           "San Francisco",
				"address":        "1 Main St",
				"postal_code":    "94105",
				"phone_number":   "+14155550100",
				"t_shirt_size":   "M",
			})),
			wantMeta: &domain.UserMetadata{
				Name:          "Alice Example",
				GivenName:     "Alice",
				FamilyName:    "Example",
				Picture:       "https://example.com/alice.png",
				Zoneinfo:      "America/New_York",
				JobTitle:      "Engineer",
				Organization:  "LF",
				Country:       "US",
				StateProvince: "CA",
				City:          "San Francisco",
				Address:       "1 Main St",
				PostalCode:    "94105",
				PhoneNumber:   "+14155550100",
				TShirtSize:    "M",
			},
		},
		{
			name:     "partial fields — omitted fields remain zero-value",
			reply:    replyMsg(marshalSuccess(map[string]interface{}{"name": "Bob"})),
			wantMeta: &domain.UserMetadata{Name: "Bob"},
		},
		{
			name:       "success=false returns error",
			reply:      replyMsg([]byte(`{"success":false,"error":"not found"}`)),
			wantErrStr: "user metadata not found",
		},
		{
			name:       "malformed JSON returns parse error",
			reply:      replyMsg([]byte(`not-json`)),
			wantErrStr: "failed to parse user_metadata response",
		},
		{
			name:       "transport error is returned",
			reply:      nil,
			replyErr:   fmt.Errorf("nats: timeout"),
			wantErrStr: "nats: timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &MockNATSConn{}
			mockConn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *natsgo.Msg) bool {
				return msg.Subject == constants.AuthUserMetadataReadSubject
			})).Return(tt.reply, tt.replyErr)

			reader := &UserReaderNATS{NatsConn: mockConn}
			got, err := reader.UserMetadataByPrincipal(context.Background(), "alice")

			if tt.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantMeta, got)
			}
			mockConn.AssertExpectations(t)
		})
	}
}
