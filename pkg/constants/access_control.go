// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package constants contains the constants for the project service.
package constants

const (
	// AccessCheckSubject is the subject used for access control checks
	// The subject is of the form: <lfx_environment>.lfx.access_check.request
	AccessCheckSubject = ".lfx.access_check.request"
	// AnonymousPrincipal is the identifier for anonymous users
	AnonymousPrincipal = `_anonymous`
)
