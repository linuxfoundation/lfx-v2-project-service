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

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/natsreply"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	nats "github.com/nats-io/nats.go"
)

// fgaRequestTimeout is the maximum time to wait for an fga-sync response.
// A stalled fga-sync must not block a PUT /projects/:id indefinitely.
const fgaRequestTimeout = 5 * time.Second

// NATSChecker sends a NATS request/reply to fga-sync to verify an FGA relation.
// It is safe for concurrent use.
type NATSChecker struct {
	conn natsreply.Conn

	// replyWindow is how long Check keeps listening for further replies after the first.
	// Zero uses natsreply.DefaultWindow.
	replyWindow time.Duration
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
//
// The reply travels over an unauthenticated core-NATS inbox, so every reply arriving within
// the reply window of the first is collected and Check fails with natsreply.ErrConflictingReplies
// when they disagree: a rogue subscriber answering "true" ahead of fga-sync cannot authorize a
// change. This is best effort; a spoofed reply that lands before a slow genuine one is not caught.
func (c *NATSChecker) Check(ctx context.Context, user, relation, object string) (bool, error) {
	tuple := fmt.Sprintf("%s#%s@%s", object, relation, user)

	reqCtx, cancel := context.WithTimeout(ctx, fgaRequestTimeout)
	defer cancel()

	replies, err := natsreply.Collect(reqCtx, c.conn, &nats.Msg{
		Subject: constants.AccessCheckSubject,
		Data:    []byte(tuple),
	}, c.replyWindow)
	if err != nil {
		return false, fmt.Errorf("fga check nats request: %w", err)
	}

	allowed := make([]bool, len(replies))
	errs := make([]error, len(replies))
	keys := make([]string, len(replies))
	for i, reply := range replies {
		allowed[i], errs[i] = parseCheckReply(reply.Data, tuple)
		switch {
		case errs[i] != nil:
			keys[i] = "error"
		case allowed[i]:
			keys[i] = "true"
		default:
			keys[i] = "false"
		}
	}
	if !natsreply.Agree(keys) {
		return false, fmt.Errorf("fga check: %w", natsreply.ErrConflictingReplies)
	}
	return allowed[0], errs[0]
}

// parseCheckReply extracts the authoritative result for tuple from one fga-sync reply.
// Response is newline-delimited "tuple\tallowed" lines. Only a line whose
// leading tuple matches the request and whose value is "true" or "false"
// is considered authoritative. Anything else (e.g. an fga-sync error
// string with no tab separator) is not a valid denial — return an error so
// the caller maps this to ErrInternal rather than silently denying access.
func parseCheckReply(data []byte, tuple string) (bool, error) {
	for _, line := range bytes.Split(data, []byte("\n")) {
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
	return false, fmt.Errorf("fga check: no authoritative result for %q in response %q", tuple, data)
}
