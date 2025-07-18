// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// Constants for the HTTP request headers
const (
	// RequestIDHeader is the header name for the request ID
	RequestIDHeader string = "X-REQUEST-ID"

	// EtagHeader is the header name for the ETag
	EtagHeader string = "ETag"
)

// contextRequestID is the type for the request ID context key
type contextRequestID string

// RequestIDContextID is the context ID for the request ID
const RequestIDContextID contextRequestID = "X-REQUEST-ID"

type contextPrincipal int

// PrincipalContextID is the context ID for the principal
const PrincipalContextID contextPrincipal = iota

type contextEtag string

// ETagContextID is the context ID for the ETag
const ETagContextID contextEtag = "etag"
