// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package natsreply

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timedReply is a reply the fake connection delivers after delay.
type timedReply struct {
	data   string
	delay  time.Duration
	status string
}

// fakeConn delivers preset replies to the inbox channel once a request is published.
type fakeConn struct {
	replies    []timedReply
	subErr     error
	publishErr error

	mu      sync.Mutex
	ch      chan *nats.Msg
	gotMsg  *nats.Msg
	gotInbx string
}

func (f *fakeConn) NewInbox() string { return "_INBOX.test" }

func (f *fakeConn) ChanSubscribe(subj string, ch chan *nats.Msg) (*nats.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotInbx = subj
	f.ch = ch
	return nil, f.subErr
}

func (f *fakeConn) PublishMsg(msg *nats.Msg) error {
	f.mu.Lock()
	f.gotMsg = msg
	ch := f.ch
	f.mu.Unlock()
	if f.publishErr != nil {
		return f.publishErr
	}
	for _, r := range f.replies {
		go func() {
			time.Sleep(r.delay)
			m := &nats.Msg{Data: []byte(r.data), Header: nats.Header{}}
			if r.status != "" {
				m.Header.Set("Status", r.status)
			}
			ch <- m
		}()
	}
	return nil
}

func TestCollect(t *testing.T) {
	tests := []struct {
		name       string
		replies    []timedReply
		subErr     error
		publishErr error
		ctxTimeout time.Duration
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
			name:    "no-responders status becomes ErrNoResponders",
			replies: []timedReply{{status: "503"}},
			wantErr: nats.ErrNoResponders,
		},
		{
			name:       "no reply before the context deadline returns the context error",
			ctxTimeout: 50 * time.Millisecond,
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
			conn := &fakeConn{replies: tt.replies, subErr: tt.subErr, publishErr: tt.publishErr}
			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}

			got, err := Collect(ctx, conn, nats.NewMsg("test.subject"), 100*time.Millisecond)

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

func TestAgree(t *testing.T) {
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
			assert.Equal(t, tt.want, Agree(tt.keys))
		})
	}
}
