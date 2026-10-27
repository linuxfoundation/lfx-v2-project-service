// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package natsreply collects every reply to a core-NATS request instead of trusting the
// first one. Core NATS carries no verified sender identity, so a rogue subscriber on a
// request subject can answer before the real responder. Callers compare the collected
// replies and fail closed when they disagree.
package natsreply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	// DefaultWindow is how long Collect keeps listening after the first reply arrives.
	DefaultWindow = 100 * time.Millisecond

	// defaultTimeout bounds the wait for a first reply when the caller's context has no deadline.
	defaultTimeout = 5 * time.Second

	// replyBuffer is the capacity of the inbox channel.
	replyBuffer = 16

	// maxReplies caps how many replies Collect accepts. The request subjects are answered from
	// a queue group, so a legitimate lookup yields one reply; more than this is a flood.
	maxReplies = 4

	// statusHdr and noRespondersStatus mirror the unexported nats.go constants the server uses
	// to say a request had no subscriber.
	statusHdr          = "Status"
	noRespondersStatus = "503"
)

// ErrConflictingReplies is returned when responders disagree on the answer to a request.
var ErrConflictingReplies = errors.New("conflicting replies from responders")

// Conn is the subset of *nats.Conn that Collect needs.
type Conn interface {
	NewInbox() string
	ChanSubscribe(subj string, ch chan *nats.Msg) (*nats.Subscription, error)
	PublishMsg(msg *nats.Msg) error
}

// Collect publishes msg with a private reply inbox and returns every reply received until
// window has elapsed since the first one. It blocks until a first reply arrives or ctx ends.
// A non-positive window uses DefaultWindow. If ctx has no deadline, a default timeout applies.
// A no-responders status reply is reported as nats.ErrNoResponders, as a plain request would.
//
// The inbox subject travels in the request, so a rogue subscriber can flood it. Collect fails
// closed with ErrConflictingReplies when more than maxReplies arrive or the client dropped any
// reply, instead of trusting whatever survived: a burst of identical spoofs must not be able to
// push the genuine reply out of the buffer and leave only agreeing answers.
func Collect(ctx context.Context, conn Conn, msg *nats.Msg, window time.Duration) ([]*nats.Msg, error) {
	if window <= 0 {
		window = DefaultWindow
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	inbox := conn.NewInbox()
	ch := make(chan *nats.Msg, replyBuffer)
	sub, err := conn.ChanSubscribe(inbox, ch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sub.Unsubscribe() }()

	msg.Reply = inbox
	if err := conn.PublishMsg(msg); err != nil {
		return nil, err
	}

	var replies []*nats.Msg
	select {
	case first := <-ch:
		if isNoResponders(first) {
			return nil, nats.ErrNoResponders
		}
		replies = append(replies, first)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	timer := time.NewTimer(window)
	defer timer.Stop()
collect:
	for {
		select {
		case m := <-ch:
			if isNoResponders(m) {
				continue
			}
			if len(replies) == maxReplies {
				return nil, ErrConflictingReplies
			}
			replies = append(replies, m)
		case <-timer.C:
			break collect
		case <-ctx.Done():
			break collect
		}
	}

	if dropped, err := sub.Dropped(); err == nil && dropped > 0 {
		return nil, ErrConflictingReplies
	}
	return replies, nil
}

// Conflict builds the error callers return when collected replies disagree. It names the
// subject, the reply count and how many distinct outcomes there were, so a spoofing attempt is
// distinguishable from a transport failure in the callers' existing logs, without carrying any
// reply content (usernames and emails are PII).
func Conflict(subject string, keys []string) error {
	distinct := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		distinct[k] = struct{}{}
	}
	return fmt.Errorf("%w on %s: %d replies, %d distinct outcomes", ErrConflictingReplies, subject, len(keys), len(distinct))
}

// Agree reports whether every outcome key is identical. An empty slice agrees.
func Agree(keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	for _, k := range keys[1:] {
		if k != keys[0] {
			return false
		}
	}
	return true
}

func isNoResponders(m *nats.Msg) bool {
	return len(m.Data) == 0 && m.Header.Get(statusHdr) == noRespondersStatus
}
