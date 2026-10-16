// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
)

const (
	uid1 = "00000000-0000-4000-8000-000000000001"
	uid2 = "00000000-0000-4000-8000-000000000002"
	uid3 = "00000000-0000-4000-8000-000000000003"
)

// unreachableNATS points at a closed port so a command's Run gets as far as the
// connect and fails there.
func unreachableNATS() natsinfra.Config {
	return natsinfra.Config{URL: "nats://127.0.0.1:1", Timeout: 200 * time.Millisecond}
}

type fakeEntry struct {
	jetstream.KeyValueEntry
	key   string
	value []byte
}

func (e fakeEntry) Key() string   { return e.key }
func (e fakeEntry) Value() []byte { return e.value }

type fakeKeyLister struct{ keys []string }

func (l fakeKeyLister) Keys() <-chan string {
	ch := make(chan string, len(l.keys))
	for _, k := range l.keys {
		ch <- k
	}
	close(ch)
	return ch
}

func (l fakeKeyLister) Stop() error { return nil }

// fakeKV is an in-memory projects bucket. A key in vanished is listed but gone
// by the time it is read, as when a record is deleted mid-scan.
type fakeKV struct {
	values    map[string]string
	vanished  map[string]bool
	listErr   error
	getErrFor string
}

func (f *fakeKV) ListKeys(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	keys := make([]string, 0, len(f.values)+len(f.vanished))
	for k := range f.values {
		keys = append(keys, k)
	}
	for k := range f.vanished {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fakeKeyLister{keys: keys}, nil
}

func (f *fakeKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == f.getErrFor {
		return nil, fmt.Errorf("boom")
	}
	if v, ok := f.values[key]; ok {
		return fakeEntry{key: key, value: []byte(v)}, nil
	}
	return nil, jetstream.ErrKeyNotFound
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// newOpenSearchClient answers every search with the given hit IDs.
func newOpenSearchClient(t *testing.T, hitIDs ...string) *opensearchgo.Client {
	t.Helper()
	body := `{"hits":{"hits":[`
	for i, id := range hitIDs {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"_id":%q}`, id)
	}
	body += `]}}`
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}, nil
		}),
		DisableRetry: true,
	})
	require.NoError(t, err)
	return client
}

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()

	assert.Equal(t, "inspect", cmd.Name())
	assert.NotEmpty(t, cmd.Help())

	subs := cmd.Subcommands()
	names := make([]string, 0, len(subs))
	for key, sub := range subs {
		assert.Equal(t, key, sub.Name(), "the registry key must match the subcommand's own name")
		assert.NotEmpty(t, sub.Help(), "%s needs help text", key)
		names = append(names, key)
	}
	assert.ElementsMatch(t, []string{"project", "slug-index", "orphans", "index-drift", "settings-access", "children", "parents"}, names)
}

func TestWriteReport(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeReport(&buf, map[string]int{"a": 1}))
	assert.JSONEq(t, `{"a":1}`, buf.String())

	assert.Error(t, writeReport(&buf, map[string]any{"c": make(chan int)}))
}

func TestProblemsError(t *testing.T) {
	assert.NoError(t, problemsError("inspect x", 0))
	err := problemsError("inspect x", 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspect x found 3 problem(s)")
}

func TestSubcommandRun_flagsAndConnect(t *testing.T) {
	rc := func(args ...string) commands.RunContext {
		return commands.RunContext{Args: args, NATSConfig: unreachableNATS()}
	}
	tests := []struct {
		name    string
		sub     commands.Subcommand
		args    []string
		wantErr string
	}{
		{name: "slug-index rejects extra args", sub: &slugIndexSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "slug-index fails at the NATS connect", sub: &slugIndexSubcommand{}, wantErr: ""},
		{name: "orphans rejects extra args", sub: &orphansSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "orphans fails at the NATS connect", sub: &orphansSubcommand{}, wantErr: ""},
		{name: "index-drift rejects extra args", sub: &indexDriftSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "index-drift fails at the NATS connect", sub: &indexDriftSubcommand{}, wantErr: ""},
		{name: "settings-access rejects extra args", sub: &settingsAccessSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "settings-access rejects a bad concurrency", sub: &settingsAccessSubcommand{}, args: []string{"--concurrency=0"}, wantErr: "concurrency must be between 1 and 100"},
		{name: "children rejects extra args", sub: &childrenSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "children fails at the NATS connect", sub: &childrenSubcommand{}, wantErr: ""},
		{name: "parents rejects extra args", sub: &parentsSubcommand{}, args: []string{"extra"}, wantErr: "unexpected argument"},
		{name: "parents fails at the NATS connect", sub: &parentsSubcommand{}, wantErr: ""},
		{name: "project needs one argument", sub: &projectSubcommand{}, wantErr: "exactly one project UID or slug"},
		{name: "project rejects two arguments", sub: &projectSubcommand{}, args: []string{"a", "b"}, wantErr: "exactly one project UID or slug"},
		{name: "project rejects a blank target", sub: &projectSubcommand{}, args: []string{"  "}, wantErr: "must not be empty"},
		{name: "project fails at the NATS connect", sub: &projectSubcommand{}, args: []string{"--skip-fga", "--skip-opensearch", "example"}, wantErr: ""},
		{name: "help is not an error", sub: &projectSubcommand{}, args: []string{"-h"}, wantErr: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sub.Run(context.Background(), rc(tt.args...))
			switch tt.wantErr {
			case "-":
				assert.NoError(t, err)
			case "":
				require.Error(t, err, "the unreachable NATS server must fail the run")
			default:
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
