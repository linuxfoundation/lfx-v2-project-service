// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package fga

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRequester is a fake requester that returns a preset response without a
// live NATS server.
type stubRequester struct {
	response []byte
	err      error
	gotSubj  string
	gotData  string
}

func (s *stubRequester) RequestMsgWithContext(_ context.Context, msg *nats.Msg) (*nats.Msg, error) {
	s.gotSubj = msg.Subject
	s.gotData = string(msg.Data)
	if s.err != nil {
		return nil, s.err
	}
	return &nats.Msg{Data: s.response}, nil
}

func TestNATSChecker_Check(t *testing.T) {
	const (
		user     = "user:alice"
		relation = "writer"
		object   = "project:abc"
	)
	tuple := fmt.Sprintf("%s#%s@%s", object, relation, user)

	tests := []struct {
		name      string
		response  []byte
		natsErr   error
		wantAllow bool
		wantErr   bool
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
			name:    "nats error propagated",
			natsErr: errors.New("no responders"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubRequester{response: tt.response, err: tt.natsErr}
			checker := &NATSChecker{conn: stub}

			got, err := checker.Check(context.Background(), user, relation, object)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAllow, got)

			if tt.natsErr == nil {
				assert.Equal(t, fgaconstants.AccessCheckSubject, stub.gotSubj)
				assert.Equal(t, tuple, stub.gotData)
			}
		})
	}
}

func TestNATSChecker_Check_Timeout(t *testing.T) {
	// Verify that the checker applies fgaRequestTimeout even when the caller
	// context has no deadline of its own.
	slow := &slowRequester{delay: fgaRequestTimeout + 100*time.Millisecond}
	checker := &NATSChecker{conn: slow}

	start := time.Now()
	_, err := checker.Check(context.Background(), "user:alice", "writer", "project:abc")
	elapsed := time.Since(start)

	require.Error(t, err, "expected timeout error")
	assert.Less(t, elapsed, fgaRequestTimeout+500*time.Millisecond,
		"should have timed out near fgaRequestTimeout")
}

type slowRequester struct{ delay time.Duration }

func (s *slowRequester) RequestMsgWithContext(ctx context.Context, _ *nats.Msg) (*nats.Msg, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
		return &nats.Msg{}, nil
	}
}
