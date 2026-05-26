// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	Picture       *string `json:"picture,omitempty"`
	Zoneinfo      *string `json:"zoneinfo,omitempty"`
	Name          *string `json:"name,omitempty"`
	GivenName     *string `json:"given_name,omitempty"`
	FamilyName    *string `json:"family_name,omitempty"`
	JobTitle      *string `json:"job_title,omitempty"`
	Organization  *string `json:"organization,omitempty"`
	Country       *string `json:"country,omitempty"`
	StateProvince *string `json:"state_province,omitempty"`
	City          *string `json:"city,omitempty"`
	Address       *string `json:"address,omitempty"`
	PostalCode    *string `json:"postal_code,omitempty"`
	PhoneNumber   *string `json:"phone_number,omitempty"`
	TShirtSize    *string `json:"t_shirt_size,omitempty"`
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
		if response.Error != "" {
			return nil, fmt.Errorf("user metadata not found: %s", response.Error)
		}
		return nil, fmt.Errorf("user metadata not found")
	}

	d := response.Data
	result := &domain.UserMetadata{}
	if d.Picture != nil {
		result.Picture = *d.Picture
	}
	if d.Zoneinfo != nil {
		result.Zoneinfo = *d.Zoneinfo
	}
	if d.Name != nil {
		result.Name = *d.Name
	}
	if d.GivenName != nil {
		result.GivenName = *d.GivenName
	}
	if d.FamilyName != nil {
		result.FamilyName = *d.FamilyName
	}
	if d.JobTitle != nil {
		result.JobTitle = *d.JobTitle
	}
	if d.Organization != nil {
		result.Organization = *d.Organization
	}
	if d.Country != nil {
		result.Country = *d.Country
	}
	if d.StateProvince != nil {
		result.StateProvince = *d.StateProvince
	}
	if d.City != nil {
		result.City = *d.City
	}
	if d.Address != nil {
		result.Address = *d.Address
	}
	if d.PostalCode != nil {
		result.PostalCode = *d.PostalCode
	}
	if d.PhoneNumber != nil {
		result.PhoneNumber = *d.PhoneNumber
	}
	if d.TShirtSize != nil {
		result.TShirtSize = *d.TShirtSize
	}
	return result, nil
}

// SubByEmail resolves the subject identifier for the given primary email address.
// The auth service replies with a plain-text subject on success, or a JSON error envelope on miss.
func (u *UserReaderNATS) SubByEmail(ctx context.Context, email string) (string, error) {
	reply, err := u.NatsConn.RequestMsgWithContext(ctx, &natsgo.Msg{
		Subject: constants.AuthEmailToSubSubject,
		Data:    []byte(email),
	})
	if err != nil {
		return "", fmt.Errorf("email_to_sub request failed: %w", err)
	}

	// The auth service sends a plain-text subject on success and a JSON error envelope on miss.
	// Trim leading/trailing whitespace before inspection so intermediaries that add a trailing
	// newline or leading space don't corrupt the subject or bypass JSON detection.
	body := strings.TrimSpace(string(reply.Data))
	if body == "" {
		return "", domain.ErrUserNotFound
	}

	// Any object-shaped response is an envelope — parse it to distinguish an explicit
	// not-found from a malformed or unexpected reply. Returning ErrUserNotFound for
	// non-404 cases would silently clear stored principals in callers that treat
	// ErrUserNotFound as "member disappeared".
	if body[0] == '{' {
		var envelope struct {
			Success *bool  `json:"success"`
			Error   string `json:"error,omitempty"`
		}
		if err := json.Unmarshal(reply.Data, &envelope); err != nil {
			return "", fmt.Errorf("failed to parse email_to_sub response: %w", err)
		}
		if envelope.Success == nil {
			return "", fmt.Errorf("email_to_sub response missing success field")
		}
		if !*envelope.Success {
			return "", domain.ErrUserNotFound
		}
		return "", fmt.Errorf("unexpected email_to_sub success envelope")
	}

	return body, nil
}
