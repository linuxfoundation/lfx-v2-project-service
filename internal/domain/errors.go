// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "errors"

// Domain errors
var (
	// ErrProjectNotFound is returned when a project is not found.
	ErrProjectNotFound = errors.New("project not found")
	// ErrInvalidParentProject is returned when a parent project is invalid.
	ErrInvalidParentProject = errors.New("invalid parent project")
	// ErrProjectSlugExists is returned when a project slug already exists.
	ErrProjectSlugExists = errors.New("project slug already exists")
	// ErrInternal is returned when an internal error occurs.
	ErrInternal = errors.New("internal error")
	// ErrRevisionMismatch is returned when a revision mismatch occurs.
	ErrRevisionMismatch = errors.New("revision mismatch")
	// ErrUnmarshal is returned when an unmarshal error occurs.
	ErrUnmarshal = errors.New("unmarshal error")
	// ErrServiceUnavailable is returned when a service is unavailable.
	ErrServiceUnavailable = errors.New("service unavailable")
	// ErrValidationFailed is returned when a validation failed.
	ErrValidationFailed = errors.New("validation failed")
)
