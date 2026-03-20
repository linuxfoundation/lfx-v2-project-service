// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	membershipservice "github.com/linuxfoundation/lfx-v2-member-service/gen/membership_service"
	pkgerrors "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// errNotFound constructs a domain NotFound error with the given message. Used
// inline in handler methods to produce a consistent 404 response via wrapError.
func errNotFound(msg string) error {
	return pkgerrors.NewNotFound(msg, fmt.Errorf("handler assertion"))
}

func wrapError(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "request failed",
		"error", err,
	)

	var notFound pkgerrors.NotFound
	if errors.As(err, &notFound) {
		return &membershipservice.NotFoundError{
			Message: err.Error(),
		}
	}

	var validation pkgerrors.Validation
	if errors.As(err, &validation) {
		return &membershipservice.BadRequestError{
			Message: err.Error(),
		}
	}

	var serviceUnavailable pkgerrors.ServiceUnavailable
	if errors.As(err, &serviceUnavailable) {
		return &membershipservice.ServiceUnavailableError{
			Message: err.Error(),
		}
	}

	return &membershipservice.InternalServerError{
		Message: err.Error(),
	}
}
