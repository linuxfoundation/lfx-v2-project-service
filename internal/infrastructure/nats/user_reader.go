// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/codes"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// validLFIDUsername is the accepted shape of an LFID username returned by the auth service:
// ASCII letters, digits, '.', '_' and '-', starting with an alphanumeric, at most 128 characters.
var validLFIDUsername = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// userMetadataNATSResponse is the response envelope from lfx.auth-service.user_metadata.read.
type userMetadataNATSResponse struct {
	Success bool                      `json:"success"`
	Error   string                    `json:"error,omitempty"`
	Data    *userMetadataNATSDataBody `json:"data,omitempty"`
}

// userMetadataNATSDataBody holds the profile fields the project service reads from the
// auth-service response. The auth service returns many more fields; only the four used
// for display-name resolution and audit stamping are decoded here.
type userMetadataNATSDataBody struct {
	Name       *string `json:"name,omitempty"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	Picture    *string `json:"picture,omitempty"`
}

// UserReaderNATS implements domain.UserReader via NATS requests to the auth service.
type UserReaderNATS struct {
	NatsConn INatsConn
}

// UserMetadataByPrincipal retrieves profile metadata for a user from the auth service by principal.
func (u *UserReaderNATS) UserMetadataByPrincipal(ctx context.Context, principal string) (*domain.UserMetadata, error) {
	_, span, reply, err := requestWithSpan(ctx, u.NatsConn, constants.AuthUserMetadataReadSubject, []byte(principal))
	defer span.End()
	if err != nil {
		return nil, err
	}

	var response userMetadataNATSResponse
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("failed to parse user_metadata response: %w", err)
	}

	if !response.Success || response.Data == nil {
		if response.Error != "" {
			err := fmt.Errorf("user metadata not found: %s", response.Error)
			recordSpanError(span, err)
			return nil, err
		}
		err := fmt.Errorf("user metadata not found")
		recordSpanError(span, err)
		return nil, err
	}
	span.SetStatus(codes.Ok, "")

	d := response.Data
	result := &domain.UserMetadata{}
	if d.Name != nil {
		result.Name = *d.Name
	}
	if d.GivenName != nil {
		result.GivenName = *d.GivenName
	}
	if d.FamilyName != nil {
		result.FamilyName = *d.FamilyName
	}
	if d.Picture != nil {
		result.Picture = *d.Picture
	}
	return result, nil
}

// UsernameByEmail resolves the registered LFID username for the given primary email address.
// The auth service replies with a plain-text username on success, or a JSON error envelope on miss.
func (u *UserReaderNATS) UsernameByEmail(ctx context.Context, email string) (string, error) {
	_, span, reply, err := requestWithSpan(ctx, u.NatsConn, constants.AuthEmailToUsernameSubject, []byte(email))
	defer span.End()
	if err != nil {
		return "", fmt.Errorf("email_to_username request failed: %w", err)
	}

	// The auth service sends a plain-text username on success and a JSON error envelope on miss.
	// Trim leading/trailing whitespace before inspection so intermediaries that add a trailing
	// newline or leading space don't corrupt the username or bypass JSON detection.
	body := strings.TrimSpace(string(reply.Data))
	if body == "" {
		recordSpanError(span, domain.ErrUserNotFound)
		return "", domain.ErrUserNotFound
	}

	// Any object-shaped response is an envelope — parse it to distinguish an explicit
	// not-found from a malformed or unexpected reply. Returning ErrUserNotFound for
	// non-404 cases would silently clear stored usernames in callers that treat
	// ErrUserNotFound as "member disappeared".
	if body[0] == '{' {
		var envelope struct {
			Success *bool  `json:"success"`
			Error   string `json:"error,omitempty"`
		}
		if err := json.Unmarshal(reply.Data, &envelope); err != nil {
			recordSpanError(span, err)
			return "", fmt.Errorf("failed to parse email_to_username response: %w", err)
		}
		if envelope.Success == nil {
			err := fmt.Errorf("email_to_username response missing success field")
			recordSpanError(span, err)
			return "", err
		}
		if !*envelope.Success {
			recordSpanError(span, domain.ErrUserNotFound)
			return "", domain.ErrUserNotFound
		}
		err := fmt.Errorf("unexpected email_to_username success envelope")
		recordSpanError(span, err)
		return "", err
	}

	// The reply travels over an unauthenticated core-NATS inbox, so reject anything that is
	// not shaped like an LFID before it can be persisted or published as an FGA principal.
	if !validLFIDUsername.MatchString(body) {
		err := fmt.Errorf("email_to_username response is not a valid username")
		recordSpanError(span, err)
		return "", err
	}

	span.SetStatus(codes.Ok, "")
	return body, nil
}

type userEmailsRequest struct {
	User struct {
		AuthToken string `json:"auth_token"`
	} `json:"user"`
}

type userEmailsResponse struct {
	Success *bool  `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    struct {
		PrimaryEmail string `json:"primary_email"`
	} `json:"data"`
}

// PrimaryEmailByUsername resolves the user's primary email via auth-service user_emails.read.
func (u *UserReaderNATS) PrimaryEmailByUsername(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", nil
	}

	body := userEmailsRequest{}
	body.User.AuthToken = username
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user_emails request: %w", err)
	}

	_, span, reply, err := requestWithSpan(ctx, u.NatsConn, constants.AuthUserEmailsReadSubject, payload)
	defer span.End()
	if err != nil {
		return "", fmt.Errorf("user_emails request failed: %w", err)
	}

	var emails userEmailsResponse
	if err := json.Unmarshal(reply.Data, &emails); err != nil {
		recordSpanError(span, err)
		return "", fmt.Errorf("failed to parse user_emails response: %w", err)
	}
	if emails.Success == nil || !*emails.Success {
		return "", nil
	}

	span.SetStatus(codes.Ok, "")
	return strings.TrimSpace(emails.Data.PrimaryEmail), nil
}
