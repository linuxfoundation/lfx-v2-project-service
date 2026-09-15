// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package events

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrRPCNotFound is returned by ParseRPCError when the remote handler replied
// with code RPCErrorNotFound.  Callers can use errors.Is to distinguish a
// genuine absence from a transient failure without re-parsing the JSON.
var ErrRPCNotFound = errors.New("project-service rpc: not found")

// ErrRPCInternal is returned by ParseRPCError when the remote handler replied
// with code RPCErrorInternal (or any unrecognised code).
var ErrRPCInternal = errors.New("project-service rpc: internal error")

// The request and reply payloads for the project lookups served over NATS
// request/reply. They live here for the same reason the published event types
// do: a consuming service should import the definition rather than restate it.

// RPCErrorCode identifies the class of error in a NATS RPC error reply.
// Callers must treat any code they do not recognise as equivalent to Internal.
type RPCErrorCode string

const (
	// RPCErrorNotFound signals that the requested resource does not exist.
	RPCErrorNotFound RPCErrorCode = "not_found"
	// RPCErrorInternal signals a transient or infrastructure failure on the
	// service side; the caller should not interpret absence of the resource.
	RPCErrorInternal RPCErrorCode = "internal"
)

// RPCError is the JSON payload returned in a NATS RPC reply when the handler
// encounters an error.  A non-empty reply body that begins with `{"error":`
// is always an RPCError; a non-empty reply body without that key is a success
// payload.  An empty (nil) reply body is treated as an unrecoverable transport
// or dispatch error — it is never produced intentionally by a handler.
//
// Consuming services should import this type rather than redefine it so the
// contract stays in one place.
type RPCError struct {
	Code    RPCErrorCode `json:"error"`
	Message string       `json:"message,omitempty"`
}

// ParseRPCError inspects a NATS reply body and reports whether it is an error
// envelope returned by this service's handlers.
//
// It returns (ErrRPCNotFound, true) when Code is RPCErrorNotFound,
// (ErrRPCInternal, true) when Code is RPCErrorInternal or any unrecognised
// non-empty code, and (nil, false) when the body is empty, is not JSON, or
// does not carry an "error" key — i.e. when it is a normal success payload.
//
// Services that do not import this package can replicate the check with:
//
//	if len(data) > 0 && data[0] == '{' { /* check "error" key */ }
//
// For subjects whose success reply is also a JSON object (get_settings), use
// the full unmarshal form so the "error" key is not silently ignored.
func ParseRPCError(data []byte) (error, bool) {
	if len(data) == 0 {
		return nil, false
	}
	var envelope RPCError
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Code == "" {
		return nil, false
	}
	if envelope.Code == RPCErrorNotFound {
		return ErrRPCNotFound, true
	}
	return ErrRPCInternal, true
}

// ProjectSettingsSummary is the reply to lfx.projects-api.get_settings.
//
// It is the grant roster and the announcement date, and deliberately not the
// whole settings record: the mission statement, the executive director, the
// program manager and the opportunity owner have no consumer on this subject,
// and a lookup that ships them makes every caller a holder of them.
//
// Writers and Auditors are always present, empty rather than null when the
// project has none configured, so a caller reading an empty roster does not
// have to tell it apart from a field that was omitted.
type ProjectSettingsSummary struct {
	UID              string     `json:"uid"`
	AnnouncementDate *time.Time `json:"announcement_date"`
	Writers          []UserInfo `json:"writers"`
	Auditors         []UserInfo `json:"auditors"`
}

// ProjectListRequest is the request body of lfx.projects-api.list_projects.
//
// The two filters are a union, not an intersection: the reply holds every
// project at one of Stages, plus every project named in UIDs whatever its
// stage. That is what lets a caller ask one question it otherwise has to ask
// twice — "the projects at the stages I care about, and also these specific
// ones, which I need the current stage of precisely because they may have left
// those stages."
//
// At least one filter must be set. An empty request is refused rather than
// answered with every project, because the caller that meant to send a filter
// and sent none would otherwise be served a full store scan that looks like a
// successful answer.
type ProjectListRequest struct {
	Stages []string `json:"stages,omitempty"`
	UIDs   []string `json:"uids,omitempty"`
}

// ProjectRef is one entry in the lfx.projects-api.list_projects reply: a
// project identified, with the fields a caller needs to decide what it is and
// what to do about it, and no more.
type ProjectRef struct {
	UID          string `json:"uid"`
	Slug         string `json:"slug"`
	IsFoundation bool   `json:"is_foundation"`
	ParentUID    string `json:"parent_uid"`
	Stage        string `json:"stage"`
}
