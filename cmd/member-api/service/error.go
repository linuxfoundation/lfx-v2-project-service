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

// wrapError maps a domain error to the appropriate Goa service error type.
// The generated Make* helpers return *goa.ServiceError, which has a correctly
// implemented Error() method and exposes Fault/Temporary flags to callers.
func wrapError(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "request failed",
		"error", err,
	)

	var notFound pkgerrors.NotFound
	if errors.As(err, &notFound) {
		return membershipservice.MakeNotFound(err)
	}

	var validation pkgerrors.Validation
	if errors.As(err, &validation) {
		return membershipservice.MakeBadRequest(err)
	}

	var serviceUnavailable pkgerrors.ServiceUnavailable
	if errors.As(err, &serviceUnavailable) {
		return membershipservice.MakeServiceUnavailable(err)
	}

	return membershipservice.MakeInternalServerError(err)
}
