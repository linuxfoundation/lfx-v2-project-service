// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

type requestIDHeaderType string

// RequestIDHeader is the header name for the request ID
const RequestIDHeader requestIDHeaderType = "X-REQUEST-ID"

type contextID int

// PrincipalContextID
const PrincipalContextID contextID = iota

type contextAuthorization string

// AuthorizationHeader is the header name for the authorization
const AuthorizationHeader string = "authorization"

// AuthorizationContextID is the context ID for the authorization
const AuthorizationContextID contextAuthorization = "authorization"

// XOnBehalfOfHeader is the header name for the on-behalf-of principal (lowercase for NATS headers).
const XOnBehalfOfHeader string = "x-on-behalf-of"

// ServiceAccountBearer is the fallback Authorization header value used by the
// member-service when no user token is present in the context (e.g. internal writes).
const ServiceAccountBearer string = "Bearer lfx-v2-member-service"
