// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

// Reply collection for core-NATS requests whose answer drives an identity or authorization
// decision. Core NATS carries no verified sender identity, so a rogue subscriber on a request
// subject can answer before the real responder. Instead of trusting the first reply, callers
// collect every reply within a short window and fail closed when they disagree.

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const (
	// DefaultReplyWindow is how long CollectReplies keeps listening after the first reply arrives.
	DefaultReplyWindow = 100 * time.Millisecond

	// defaultTimeout bounds the wait for a first reply when the caller's context has no deadline.
	defaultTimeout = 5 * time.Second

	// replyBuffer is the capacity of the inbox channel.
	replyBuffer = 16

	// maxReplies caps how many replies CollectReplies accepts. The request subjects are answered from
	// a queue group, so a legitimate lookup yields one reply; more than this is a flood.
	maxReplies = 4

	// statusHdr and noRespondersStatus mirror the unexported nats.go constants the server uses
	// to say a request had no subscriber.
	statusHdr          = "Status"
	noRespondersStatus = "503"
)

// ErrConflictingReplies is returned when responders disagree on the answer to a request.
var ErrConflictingReplies = errors.New("conflicting replies from responders")

// ReplySubscription is the part of a subscription that CollectReplies uses once it is subscribed.
// It is an interface so tests can drive the dropped-message count, which a real subscription
// only reports after a slow-consumer drop.
type ReplySubscription interface {
	Dropped() (int, error)
	Unsubscribe() error
}

// ReplyConn is what CollectReplies needs from a connection.
type ReplyConn interface {
	NewInbox() string
	ChanSubscribe(subj string, ch chan *natsgo.Msg) (ReplySubscription, error)
	PublishMsg(msg *natsgo.Msg) error
}

// replyTransport is the subset of a core NATS connection that NewReplyConn adapts. Both
// *nats.Conn and INatsConn satisfy it.
type replyTransport interface {
	NewInbox() string
	ChanSubscribe(subj string, ch chan *natsgo.Msg) (*natsgo.Subscription, error)
	PublishMsg(msg *natsgo.Msg) error
}

// replyConnAdapter adapts a core NATS connection to ReplyConn.
type replyConnAdapter struct {
	replyTransport
}

// ChanSubscribe subscribes with the underlying connection and returns the subscription as a
// ReplySubscription.
func (c replyConnAdapter) ChanSubscribe(subj string, ch chan *natsgo.Msg) (ReplySubscription, error) {
	sub, err := c.replyTransport.ChanSubscribe(subj, ch)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// NewReplyConn adapts a core NATS connection for CollectReplies.
func NewReplyConn(conn replyTransport) ReplyConn {
	return replyConnAdapter{conn}
}

// CollectReplies publishes msg with a private reply inbox and returns every reply received until
// window has elapsed since the first one. It blocks until a first reply arrives or ctx ends.
// A non-positive window uses DefaultReplyWindow. If ctx has no deadline, a default timeout applies.
// If ctx ends before the window closes, CollectReplies returns ctx.Err() rather than a partial result.
// A no-responders status reply is reported as natsgo.ErrNoResponders, as a plain request would.
//
// The inbox subject travels in the request, so a rogue subscriber can flood it. CollectReplies fails
// closed with ErrConflictingReplies when more than maxReplies arrive or the client dropped any
// reply, instead of trusting whatever survived: a burst of identical spoofs must not be able to
// push the genuine reply out of the buffer and leave only agreeing answers.
func CollectReplies(ctx context.Context, conn ReplyConn, msg *natsgo.Msg, window time.Duration) ([]*natsgo.Msg, error) {
	if window <= 0 {
		window = DefaultReplyWindow
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	inbox := conn.NewInbox()
	ch := make(chan *natsgo.Msg, replyBuffer)
	sub, err := conn.ChanSubscribe(inbox, ch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sub.Unsubscribe() }()

	msg.Reply = inbox
	if err := conn.PublishMsg(msg); err != nil {
		return nil, err
	}

	var replies []*natsgo.Msg
	select {
	case first := <-ch:
		if isNoResponders(first) {
			return nil, natsgo.ErrNoResponders
		}
		replies = append(replies, first)
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// received counts every frame taken off the inbox after the first, including a late
	// no-responders status that is excluded from the outcome comparison: forged status frames
	// must advance the flood cap too, or a continuous stream of them would never trip it and
	// could keep the drain below running.
	received := 1

	// add records one reply, enforcing the flood cap.
	add := func(m *natsgo.Msg) error {
		received++
		if received > maxReplies {
			return ErrConflictingReplies
		}
		if isNoResponders(m) {
			return nil
		}
		replies = append(replies, m)
		return nil
	}

	timer := time.NewTimer(window)
	defer timer.Stop()
collect:
	for {
		select {
		case m := <-ch:
			if err := add(m); err != nil {
				return nil, err
			}
		case <-timer.C:
			break collect
		case <-ctx.Done():
			// The window is unfinished: a genuine reply that conflicts with what was collected
			// may still be on its way, so an ended context must not count as agreement.
			return nil, ctx.Err()
		}
	}

	// select picks at random among ready cases, so the timer can win while a reply is already
	// buffered. Read what is waiting before judging the replies.
drain:
	for {
		select {
		case m := <-ch:
			if err := add(m); err != nil {
				return nil, err
			}
		default:
			break drain
		}
	}

	if dropped, err := sub.Dropped(); err == nil && dropped > 0 {
		return nil, ErrConflictingReplies
	}
	return replies, nil
}

// ReplyConflict builds the error callers return when collected replies disagree. It names the
// subject, the reply count and how many distinct outcomes there were, so a spoofing attempt is
// distinguishable from a transport failure in the callers' existing logs, without carrying any
// reply content (usernames and emails are PII).
func ReplyConflict(subject string, keys []string) error {
	distinct := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		distinct[k] = struct{}{}
	}
	return fmt.Errorf("%w on %s: %d replies, %d distinct outcomes", ErrConflictingReplies, subject, len(keys), len(distinct))
}

// RepliesAgree reports whether every outcome key is identical. An empty slice agrees.
func RepliesAgree(keys []string) bool {
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

func isNoResponders(m *natsgo.Msg) bool {
	return len(m.Data) == 0 && m.Header.Get(statusHdr) == noRespondersStatus
}
