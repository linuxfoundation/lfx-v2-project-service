// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package errors

import "errors"

// Unexpected represents an unexpected error in the application.
type Unexpected struct {
	base
}

// Error returns the error message for Unexpected.
func (u Unexpected) Error() string {
	return u.error()
}

// NewUnexpected creates a new Unexpected error with the provided message.
func NewUnexpected(message string, err ...error) Unexpected {
	return Unexpected{
		base: base{
			message: message,
			err:     errors.Join(err...),
		},
	}
}

// ServiceUnavailable represents a service unavailability error in the application.
type ServiceUnavailable struct {
	base
}

// Error returns the error message for ServiceUnavailable.
func (su ServiceUnavailable) Error() string {
	return su.error()
}

// NewServiceUnavailable creates a new ServiceUnavailable error with the provided message.
func NewServiceUnavailable(message string, err ...error) ServiceUnavailable {
	return ServiceUnavailable{
		base: base{
			message: message,
			err:     errors.Join(err...),
		},
	}
}

// NotImplemented represents a not implemented error in the application.
type NotImplemented struct {
	base
}

// Error returns the error message for NotImplemented.
func (ni NotImplemented) Error() string {
	return ni.error()
}

// NewNotImplemented creates a new NotImplemented error with the provided message.
func NewNotImplemented(message string, err ...error) NotImplemented {
	return NotImplemented{
		base: base{
			message: message,
			err:     errors.Join(err...),
		},
	}
}
