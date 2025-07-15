// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

type requestIDHeaderType string

// RequestIDHeader is the header name for the request ID
const RequestIDHeader requestIDHeaderType = "X-REQUEST-ID"

type contextID int

// PrincipalContextID is the context ID for the principal
const PrincipalContextID contextID = iota

type contextEtag string

// ETagContextID is the context ID for the ETag
const ETagContextID contextEtag = "etag"
