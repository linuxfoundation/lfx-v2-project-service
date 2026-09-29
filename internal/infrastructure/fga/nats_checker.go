// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package fga provides a NATS-based client for checking FGA access relations
// via the fga-sync service's lfx.access_check.request request/reply subject.
package fga

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	nats "github.com/nats-io/nats.go"
)

// fgaRequestTimeout is the maximum time to wait for an fga-sync response.
// A stalled fga-sync must not block a PUT /projects/:id indefinitely.
const fgaRequestTimeout = 5 * time.Second

// requester is the subset of *nats.Conn used by NATSChecker. It is extracted
// as an interface so unit tests can inject a stub without a live NATS server.
type requester interface {
	RequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error)
}

// NATSChecker sends a NATS request/reply to fga-sync to verify an FGA relation.
// It is safe for concurrent use.
type NATSChecker struct {
	conn requester
}

// NewNATSChecker returns a NATSChecker backed by the given NATS connection.
func NewNATSChecker(conn *nats.Conn) *NATSChecker {
	return &NATSChecker{conn: conn}
}

// Check returns true when the user holds the given relation on the object
// according to fga-sync. Both user and object must use the "type:id" form.
//
// The request payload uses fga-sync's wire format: "object#relation@user".
// The response is newline-delimited "object#relation@user\ttrue|false" lines;
// Check returns true when the matching line reports "true", false when it
// reports "false", and an error when no matching tuple is found (which
// distinguishes a fga-sync error reply from an authoritative denial).
func (c *NATSChecker) Check(ctx context.Context, user, relation, object string) (bool, error) {
	tuple := fmt.Sprintf("%s#%s@%s", object, relation, user)

	reqCtx, cancel := context.WithTimeout(ctx, fgaRequestTimeout)
	defer cancel()

	msg, err := c.conn.RequestMsgWithContext(reqCtx, &nats.Msg{
		Subject: constants.AccessCheckSubject,
		Data:    []byte(tuple),
	})
	if err != nil {
		return false, fmt.Errorf("fga check nats request: %w", err)
	}

	// Response is newline-delimited "tuple\tallowed" lines. Only a line whose
	// leading tuple matches the request and whose value is "true" or "false"
	// is considered authoritative. Anything else (e.g. an fga-sync error
	// string with no tab separator) is not a valid denial — return an error so
	// the caller maps this to ErrInternal rather than silently denying access.
	for _, line := range bytes.Split(msg.Data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		parts := strings.SplitN(string(line), "\t", 2)
		if len(parts) != 2 || parts[0] != tuple {
			continue
		}
		switch parts[1] {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, fmt.Errorf("fga check: no authoritative result for %q in response %q", tuple, msg.Data)
}
