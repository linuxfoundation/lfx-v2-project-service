// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/natsreply"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubConn is a fake natsreply.Conn that answers a published request with preset reply
// bodies on the request's inbox channel, without a live NATS server. Replies after the first
// are delivered after a short delay so they land inside the collection window.
type stubConn struct {
	responses [][]byte
	err       error
	gotSubj   string
	gotData   string
	ch        chan *nats.Msg
}

func (s *stubConn) NewInbox() string { return "_INBOX.test" }

func (s *stubConn) ChanSubscribe(_ string, ch chan *nats.Msg) (*nats.Subscription, error) {
	s.ch = ch
	return nil, nil
}

func (s *stubConn) PublishMsg(msg *nats.Msg) error {
	s.gotSubj = msg.Subject
	s.gotData = string(msg.Data)
	if s.err != nil {
		return s.err
	}
	for i, r := range s.responses {
		delay := time.Duration(0)
		if i > 0 {
			delay = 20 * time.Millisecond
		}
		go func() {
			time.Sleep(delay)
			s.ch <- &nats.Msg{Data: r}
		}()
	}
	return nil
}

func TestNATSChecker_Check(t *testing.T) {
	const (
		user     = "user:alice"
		relation = "writer"
		object   = "project:abc"
	)
	tuple := fmt.Sprintf("%s#%s@%s", object, relation, user)

	tests := []struct {
		name         string
		response     []byte
		extra        [][]byte // further replies from other responders
		natsErr      error
		wantAllow    bool
		wantErr      bool
		wantConflict bool
	}{
		{
			name:      "allowed — matching line reports true",
			response:  []byte(tuple + "\ttrue\n"),
			wantAllow: true,
		},
		{
			name:      "denied — matching line reports false",
			response:  []byte(tuple + "\tfalse\n"),
			wantAllow: false,
		},
		{
			name:     "error — no matching tuple in response (fga-sync error string)",
			response: []byte("internal error from fga-sync\n"),
			wantErr:  true,
		},
		{
			name:     "error — empty response body",
			response: []byte(""),
			wantErr:  true,
		},
		{
			name:     "error — malformed lines only, no authoritative result",
			response: []byte("not-a-valid-line\nproject:other#writer@user:alice\ttrue\n"),
			wantErr:  true,
		},
		{
			name:      "allowed — unordered response with extra lines",
			response:  []byte("project:other#writer@user:alice\tfalse\n" + tuple + "\ttrue\n"),
			wantAllow: true,
		},
		{
			name:         "error — spoofed true against genuine false is a conflict",
			response:     []byte(tuple + "\ttrue\n"),
			extra:        [][]byte{[]byte(tuple + "\tfalse\n")},
			wantErr:      true,
			wantConflict: true,
		},
		{
			name:         "error — spoofed garbage against genuine true is a conflict",
			response:     []byte("garbage\n"),
			extra:        [][]byte{[]byte(tuple + "\ttrue\n")},
			wantErr:      true,
			wantConflict: true,
		},
		{
			name:      "allowed — identical replies from several responders agree",
			response:  []byte(tuple + "\ttrue\n"),
			extra:     [][]byte{[]byte(tuple + "\ttrue\n")},
			wantAllow: true,
		},
		{
			name:    "nats error propagated",
			natsErr: errors.New("no responders"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubConn{responses: append([][]byte{tt.response}, tt.extra...), err: tt.natsErr}
			checker := &NATSChecker{conn: stub, replyWindow: 100 * time.Millisecond}

			got, err := checker.Check(context.Background(), user, relation, object)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantConflict {
					assert.ErrorIs(t, err, natsreply.ErrConflictingReplies)
				}
				assert.False(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAllow, got)

			if tt.natsErr == nil {
				assert.Equal(t, constants.AccessCheckSubject, stub.gotSubj)
				assert.Equal(t, tuple, stub.gotData)
			}
		})
	}
}

func TestNATSChecker_Check_Timeout(t *testing.T) {
	// Verify that the checker applies fgaRequestTimeout even when the caller
	// context has no deadline of its own: no reply ever arrives.
	silent := &stubConn{}
	checker := &NATSChecker{conn: silent}

	start := time.Now()
	_, err := checker.Check(context.Background(), "user:alice", "writer", "project:abc")
	elapsed := time.Since(start)

	require.Error(t, err, "expected timeout error")
	assert.Less(t, elapsed, fgaRequestTimeout+500*time.Millisecond,
		"should have timed out near fgaRequestTimeout")
}
