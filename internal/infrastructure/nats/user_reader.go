// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	natsgo "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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
	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", constants.AuthUserMetadataReadSubject),
			attribute.Int("messaging.message.body.size", len(principal)),
		),
	)
	defer span.End()

	msg := natsgo.NewMsg(constants.AuthUserMetadataReadSubject)
	msg.Header = make(natsgo.Header)
	msg.Data = []byte(principal)
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))

	reply, err := u.NatsConn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var response userMetadataNATSResponse
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to parse user_metadata response: %w", err)
	}

	if !response.Success || response.Data == nil {
		if response.Error != "" {
			err := fmt.Errorf("user metadata not found: %s", response.Error)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		err := fmt.Errorf("user metadata not found")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetStatus(codes.Ok, "")

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

// UsernameByEmail resolves the registered LFID username for the given primary email address.
// The auth service replies with a plain-text username on success, or a JSON error envelope on miss.
func (u *UserReaderNATS) UsernameByEmail(ctx context.Context, email string) (string, error) {
	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", constants.AuthEmailToUsernameSubject),
			attribute.Int("messaging.message.body.size", len(email)),
		),
	)
	defer span.End()

	emailMsg := natsgo.NewMsg(constants.AuthEmailToUsernameSubject)
	emailMsg.Header = make(natsgo.Header)
	emailMsg.Data = []byte(email)
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(emailMsg.Header))

	reply, err := u.NatsConn.RequestMsgWithContext(ctx, emailMsg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("email_to_username request failed: %w", err)
	}

	// The auth service sends a plain-text username on success and a JSON error envelope on miss.
	// Trim leading/trailing whitespace before inspection so intermediaries that add a trailing
	// newline or leading space don't corrupt the username or bypass JSON detection.
	body := strings.TrimSpace(string(reply.Data))
	if body == "" {
		span.RecordError(domain.ErrUserNotFound)
		span.SetStatus(codes.Error, domain.ErrUserNotFound.Error())
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
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", fmt.Errorf("failed to parse email_to_username response: %w", err)
		}
		if envelope.Success == nil {
			err := fmt.Errorf("email_to_username response missing success field")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return "", err
		}
		if !*envelope.Success {
			span.RecordError(domain.ErrUserNotFound)
			span.SetStatus(codes.Error, domain.ErrUserNotFound.Error())
			return "", domain.ErrUserNotFound
		}
		err := fmt.Errorf("unexpected email_to_username success envelope")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	span.SetStatus(codes.Ok, "")
	return body, nil
}
