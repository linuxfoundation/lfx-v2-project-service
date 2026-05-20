// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// userReader provides NATS RPC-based implementation of port.UserReader.
type userReader struct {
	client *NATSClient
}

// NewUserReader creates a new UserReader that resolves user information via
// NATS RPC calls to auth-service.
func NewUserReader(client *NATSClient) port.UserReader {
	return &userReader{client: client}
}

// SubByEmail resolves an email address to its OIDC subject identifier via
// auth-service NATS RPC. Returns NotFound if the email does not exist or the
// RPC fails.
func (u *userReader) SubByEmail(ctx context.Context, email string) (string, error) {
	data := []byte(email)
	msg, err := u.client.Conn().RequestWithContext(ctx, constants.AuthEmailToSubLookupSubject, data)
	if err != nil {
		return "", errors.NewNotFound(fmt.Sprintf("user sub not found for email: %s", email), err)
	}

	response := string(msg.Data)
	if response == "" {
		return "", errors.NewNotFound(fmt.Sprintf("user sub not found for email: %s", email))
	}

	var errorMessage ErrorMessageNATSResponse
	if err := errorMessage.CheckError(response); err != nil {
		return "", err
	}

	return response, nil
}
