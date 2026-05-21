// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"

	natsgo "github.com/nats-io/nats.go"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// userMetadataNATSResponse is the response envelope from lfx.auth-service.user_metadata.read.
type userMetadataNATSResponse struct {
	Success bool                      `json:"success"`
	Error   string                    `json:"error,omitempty"`
	Data    *userMetadataNATSDataBody `json:"data,omitempty"`
}

// userMetadataNATSDataBody holds the profile fields from the auth-service response.
type userMetadataNATSDataBody struct {
	Name       *string `json:"name,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
}

// UserReaderNATS implements domain.UserReader via NATS requests to the auth service.
type UserReaderNATS struct {
	NatsConn INatsConn
}

// UserMetadataByPrincipal retrieves profile metadata for a user from the auth service by principal.
func (u *UserReaderNATS) UserMetadataByPrincipal(ctx context.Context, principal string) (*domain.UserMetadata, error) {
	reply, err := u.NatsConn.RequestMsgWithContext(ctx, &natsgo.Msg{
		Subject: constants.AuthUserMetadataReadSubject,
		Data:    []byte(principal),
	})
	if err != nil {
		return nil, err
	}

	var response userMetadataNATSResponse
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse user_metadata response: %w", err)
	}

	if !response.Success || response.Data == nil {
		return nil, fmt.Errorf("user metadata not found for principal")
	}

	result := &domain.UserMetadata{}
	if response.Data.Name != nil {
		result.Name = *response.Data.Name
	}
	if response.Data.GivenName != nil {
		result.GivenName = *response.Data.GivenName
	}
	if response.Data.FamilyName != nil {
		result.FamilyName = *response.Data.FamilyName
	}
	return result, nil
}

// emailToUsernameErrorResponse mirrors the auth-service error envelope for lfx.auth-service.email_to_username.
// The type is internal to the auth service, so we keep a local copy of the relevant fields.
type emailToUsernameErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// UsernameByEmail resolves the registered LFID username for the given primary email address.
// The auth service replies with a plain-text username on success, or a JSON error envelope on miss.
func (u *UserReaderNATS) UsernameByEmail(ctx context.Context, email string) (string, error) {
	reply, err := u.NatsConn.RequestMsgWithContext(ctx, &natsgo.Msg{
		Subject: constants.AuthEmailToUsernameSubject,
		Data:    []byte(email),
	})
	if err != nil {
		return "", fmt.Errorf("email_to_username request failed: %w", err)
	}

	// The auth service sends a plain-text username on success and a JSON error envelope on miss.
	// Only attempt JSON decode when the body starts with '{' to avoid misinterpreting a valid
	// plain-text username that happens to be valid JSON (e.g. a numeric string).
	if len(reply.Data) > 0 && reply.Data[0] == '{' {
		var errResp emailToUsernameErrorResponse
		if json.Unmarshal(reply.Data, &errResp) == nil && !errResp.Success {
			return "", domain.ErrUserNotFound
		}
	}

	username := string(reply.Data)
	if username == "" {
		return "", domain.ErrUserNotFound
	}

	return username, nil
}
