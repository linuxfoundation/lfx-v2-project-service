// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timedReply is a reply the fake connection delivers after delay.
type timedReply struct {
	data   string
	delay  time.Duration
	status string
}

// fakeReplySub is a ReplySubscription that reports a preset dropped count and records Unsubscribe.
type fakeReplySub struct {
	dropped      int
	droppedErr   error
	unsubscribed bool
}

func (f *fakeReplySub) Dropped() (int, error) { return f.dropped, f.droppedErr }

func (f *fakeReplySub) Unsubscribe() error {
	f.unsubscribed = true
	return nil
}

// fakeReplyConn delivers preset replies to the inbox channel once a request is published.
type fakeReplyConn struct {
	replies    []timedReply
	subErr     error
	publishErr error
	sub        fakeReplySub
	inline     bool // deliver replies synchronously inside PublishMsg

	mu      sync.Mutex
	ch      chan *natsgo.Msg
	gotMsg  *natsgo.Msg
	gotInbx string
}

func (f *fakeReplyConn) NewInbox() string { return "_INBOX.test" }

func (f *fakeReplyConn) ChanSubscribe(subj string, ch chan *natsgo.Msg) (ReplySubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotInbx = subj
	f.ch = ch
	if f.subErr != nil {
		return nil, f.subErr
	}
	return &f.sub, nil
}

func (r timedReply) msg() *natsgo.Msg {
	m := &natsgo.Msg{Data: []byte(r.data), Header: natsgo.Header{}}
	if r.status != "" {
		m.Header.Set("Status", r.status)
	}
	return m
}

func (f *fakeReplyConn) PublishMsg(msg *natsgo.Msg) error {
	f.mu.Lock()
	f.gotMsg = msg
	ch := f.ch
	f.mu.Unlock()
	if f.publishErr != nil {
		return f.publishErr
	}
	for _, r := range f.replies {
		if f.inline {
			ch <- r.msg()
			continue
		}
		go func() {
			time.Sleep(r.delay)
			ch <- r.msg()
		}()
	}
	return nil
}

func TestCollectReplies(t *testing.T) {
	tests := []struct {
		name       string
		replies    []timedReply
		subErr     error
		publishErr error
		ctxTimeout time.Duration
		window     time.Duration // zero uses 100ms
		inline     bool          // deliver replies in order, synchronously
		sub        fakeReplySub  // dropped count the subscription reports
		wantData   []string
		wantErr    error
	}{
		{
			name:     "single reply returned after the window",
			replies:  []timedReply{{data: "alice"}},
			wantData: []string{"alice"},
		},
		{
			name:     "second reply inside the window is collected",
			replies:  []timedReply{{data: "mallory"}, {data: "alice", delay: 30 * time.Millisecond}},
			wantData: []string{"mallory", "alice"},
		},
		{
			name:     "reply after the window is not collected",
			replies:  []timedReply{{data: "alice"}, {data: "late", delay: 400 * time.Millisecond}},
			wantData: []string{"alice"},
		},
		{
			name:     "no-responders status after the first reply is ignored",
			replies:  []timedReply{{data: "alice"}, {status: "503", delay: 30 * time.Millisecond}},
			wantData: []string{"alice"},
		},
		{
			name:   "flood of forged no-responders frames fails closed",
			inline: true,
			replies: []timedReply{
				{data: "alice"}, {status: "503"}, {status: "503"},
				{status: "503"}, {status: "503"}, {status: "503"},
			},
			wantErr: ErrConflictingReplies,
		},
		{
			name: "flood of identical replies fails closed",
			replies: []timedReply{
				{data: "mallory"}, {data: "mallory"}, {data: "mallory"},
				{data: "mallory"}, {data: "mallory"}, {data: "mallory"},
			},
			wantErr: ErrConflictingReplies,
		},
		{
			name:     "dropped replies fail closed even when the survivors agree",
			replies:  []timedReply{{data: "mallory"}},
			sub:      fakeReplySub{dropped: 3},
			wantErr:  ErrConflictingReplies,
			wantData: nil,
		},
		{
			name:     "dropped count unavailable does not reject an otherwise clean collection",
			replies:  []timedReply{{data: "alice"}},
			sub:      fakeReplySub{droppedErr: natsgo.ErrBadSubscription},
			wantData: []string{"alice"},
		},
		{
			name:    "no-responders status becomes ErrNoResponders",
			replies: []timedReply{{status: "503"}},
			wantErr: natsgo.ErrNoResponders,
		},
		{
			name:       "no reply before the context deadline returns the context error",
			ctxTimeout: 50 * time.Millisecond,
			wantErr:    context.DeadlineExceeded,
		},
		{
			name:       "context ending after the first reply fails instead of returning a partial collection",
			replies:    []timedReply{{data: "mallory"}},
			ctxTimeout: 50 * time.Millisecond,
			window:     500 * time.Millisecond,
			wantErr:    context.DeadlineExceeded,
		},
		{
			name:    "subscribe error is returned",
			subErr:  errors.New("subscribe failed"),
			wantErr: nil,
		},
		{
			name:       "publish error is returned",
			publishErr: errors.New("connection closed"),
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &fakeReplyConn{replies: tt.replies, subErr: tt.subErr, publishErr: tt.publishErr, inline: tt.inline, sub: tt.sub}
			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}

			window := tt.window
			if window == 0 {
				window = 100 * time.Millisecond
			}
			got, err := CollectReplies(ctx, conn, natsgo.NewMsg("test.subject"), window)

			if tt.subErr == nil {
				assert.True(t, conn.sub.unsubscribed, "the inbox subscription must be released on every exit path")
			}

			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.subErr != nil || tt.publishErr != nil:
				require.Error(t, err)
			default:
				require.NoError(t, err)
				var data []string
				for _, m := range got {
					data = append(data, string(m.Data))
				}
				assert.Equal(t, tt.wantData, data)
				assert.Equal(t, "_INBOX.test", conn.gotMsg.Reply, "request must carry the private inbox")
				assert.Equal(t, "test.subject", conn.gotMsg.Subject)
			}
		})
	}
}

func TestCollectReplies_BufferedReplyAtWindowClose(t *testing.T) {
	// Both replies are already buffered when the first is read and the window is effectively
	// zero, so the timer and the inbox are ready together. select picks randomly between them;
	// repeating makes a collector that skips the buffer fail with near certainty.
	for i := 0; i < 200; i++ {
		conn := &fakeReplyConn{inline: true, replies: []timedReply{{data: "mallory"}, {data: "alice"}}}

		got, err := CollectReplies(context.Background(), conn, natsgo.NewMsg("test.subject"), time.Nanosecond)

		require.NoError(t, err)
		require.Len(t, got, 2, "iteration %d: the buffered conflicting reply must be collected", i)
	}
}

func TestCollectReplies_DefaultWindow(t *testing.T) {
	// A non-positive window falls back to DefaultReplyWindow: a second reply 30ms after the
	// first is still inside it and must be collected.
	conn := &fakeReplyConn{replies: []timedReply{{data: "mallory"}, {data: "alice", delay: 30 * time.Millisecond}}}

	got, err := CollectReplies(context.Background(), conn, natsgo.NewMsg("test.subject"), 0)

	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestCollectReplies_FloodBufferedAtWindowClose(t *testing.T) {
	// More frames than the cap are already buffered when the window closes, so the cap can trip
	// in the collection loop or in the final drain depending on which select case runs first.
	// Either way the flood must fail closed; repeating exercises both paths.
	for i := 0; i < 200; i++ {
		conn := &fakeReplyConn{inline: true, replies: []timedReply{
			{data: "mallory"}, {data: "mallory"}, {data: "mallory"},
			{data: "mallory"}, {data: "mallory"}, {data: "mallory"},
		}}

		got, err := CollectReplies(context.Background(), conn, natsgo.NewMsg("test.subject"), time.Nanosecond)

		require.ErrorIs(t, err, ErrConflictingReplies, "iteration %d", i)
		assert.Nil(t, got)
	}
}

func TestRepliesAgree(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want bool
	}{
		{name: "empty agrees", keys: nil, want: true},
		{name: "single agrees", keys: []string{"a"}, want: true},
		{name: "identical keys agree", keys: []string{"a", "a", "a"}, want: true},
		{name: "differing keys disagree", keys: []string{"a", "b"}, want: false},
		{name: "one outlier disagrees", keys: []string{"a", "a", "b"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RepliesAgree(tt.keys))
		})
	}
}

func TestReplyConflict(t *testing.T) {
	err := ReplyConflict("test.subject", []string{"u:alice", "u:mallory", "u:mallory"})

	require.ErrorIs(t, err, ErrConflictingReplies)
	assert.Contains(t, err.Error(), "test.subject")
	assert.Contains(t, err.Error(), "3 replies, 2 distinct outcomes")
	assert.NotContains(t, err.Error(), "alice", "reply content must not leak into the error")
	assert.NotContains(t, err.Error(), "mallory", "reply content must not leak into the error")
}

func TestNewReplyConn(t *testing.T) {
	t.Run("subscription is exposed as a ReplySubscription", func(t *testing.T) {
		raw := &rawReplyTransport{}
		conn := NewReplyConn(raw)

		assert.Equal(t, "_INBOX.raw", conn.NewInbox())
		sub, err := conn.ChanSubscribe("_INBOX.raw", make(chan *natsgo.Msg, 1))

		require.NoError(t, err)
		require.True(t, sub != nil, "the interface must be non-nil so Collect can call Dropped and Unsubscribe on it")
		_, dropErr := sub.Dropped()
		assert.ErrorIs(t, dropErr, natsgo.ErrBadSubscription, "a nil subscription reports a bad subscription instead of panicking")
		assert.ErrorIs(t, sub.Unsubscribe(), natsgo.ErrBadSubscription)
	})

	t.Run("subscribe error is returned with no subscription", func(t *testing.T) {
		raw := &rawReplyTransport{subErr: errors.New("subscribe failed")}

		sub, err := NewReplyConn(raw).ChanSubscribe("_INBOX.raw", make(chan *natsgo.Msg, 1))

		require.Error(t, err)
		assert.Nil(t, sub)
	})

	t.Run("publish passes through to the underlying connection", func(t *testing.T) {
		raw := &rawReplyTransport{}

		require.NoError(t, NewReplyConn(raw).PublishMsg(natsgo.NewMsg("test.subject")))

		assert.Equal(t, "test.subject", raw.published.Subject)
	})
}

// rawReplyTransport is a replyTransport returning a nil *nats.Subscription, as the test fakes
// of INatsConn do.
type rawReplyTransport struct {
	subErr    error
	published *natsgo.Msg
}

func (r *rawReplyTransport) NewInbox() string { return "_INBOX.raw" }

func (r *rawReplyTransport) ChanSubscribe(string, chan *natsgo.Msg) (*natsgo.Subscription, error) {
	return nil, r.subErr
}

func (r *rawReplyTransport) PublishMsg(msg *natsgo.Msg) error {
	r.published = msg
	return nil
}
