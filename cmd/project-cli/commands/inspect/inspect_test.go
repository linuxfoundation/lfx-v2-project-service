// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
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

// wantStatus is the status a report must carry given the run's error: a run that
// printed a report and failed did so because it found problems.
func wantStatus(err error) string {
	if err != nil {
		return statusProblems
	}
	return statusOK
}

func TestStatusFor(t *testing.T) {
	assert.Equal(t, reportStatus{Status: "ok"}, statusFor(0))
	assert.Equal(t, reportStatus{Status: "problems_found"}, statusFor(1))
}

func TestFinishReport(t *testing.T) {
	type report struct {
		reportStatus
		N int `json:"n"`
	}

	t.Run("a clean run prints the report, logs OK and returns nil", func(t *testing.T) {
		var logs bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })
		var out bytes.Buffer

		err := finishReport(context.Background(), "inspect x", &out, report{reportStatus: statusFor(0), N: 4}, 0, "4 things")

		require.NoError(t, err)
		assert.JSONEq(t, `{"status":"ok","n":4}`, out.String())
		assert.Contains(t, logs.String(), "inspect x: OK (4 things, no problems)")
	})

	t.Run("a run with problems logs the count and returns an error", func(t *testing.T) {
		var logs bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })
		var out bytes.Buffer

		err := finishReport(context.Background(), "inspect x", &out, report{reportStatus: statusFor(2)}, 2, "4 things")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "inspect x found 2 problem(s)")
		assert.JSONEq(t, `{"status":"problems_found","n":0}`, out.String())
		assert.Contains(t, logs.String(), "inspect x: 2 problem(s) found (4 things)")
	})

	t.Run("an unencodable report is an error", func(t *testing.T) {
		err := finishReport(context.Background(), "inspect x", &bytes.Buffer{}, map[string]any{"c": make(chan int)}, 0, "")
		assert.Error(t, err)
	})
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

// runCase is one Run call. wantErr "" means the run must fail at the NATS connect
// (nothing before it can fail), "-" means success, anything else is an error substring.
type runCase struct {
	name    string
	args    []string
	env     map[string]string
	wantErr string
}

// runSubcommand runs each case against sub with an unreachable NATS server.
func runSubcommand(t *testing.T, sub commands.Subcommand, cases []runCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			err := sub.Run(context.Background(), commands.RunContext{Args: tt.args, NATSConfig: unreachableNATS()})
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
