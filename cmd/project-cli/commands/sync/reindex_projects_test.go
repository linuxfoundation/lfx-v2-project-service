// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// projectEnvelopeMatcher asserts that a SendIndexerMessage call for the
// project subject carries the expected action and the project's own data.
func projectEnvelopeMatcher(action indexerConstants.MessageAction, base *models.ProjectBase) func(any) bool {
	return func(v any) bool {
		msg, ok := v.(indexerTypes.IndexerMessageEnvelope)
		if !ok {
			return false
		}
		return msg.Action == action &&
			reflect.DeepEqual(msg.Data, *base) &&
			reflect.DeepEqual(msg.IndexingConfig, base.IndexingConfig())
	}
}

// settingsEnvelopeMatcher asserts that a SendIndexerMessage call for the
// project_settings subject carries the expected action and settings data.
func settingsEnvelopeMatcher(action indexerConstants.MessageAction, base *models.ProjectBase, settings *models.ProjectSettings) func(any) bool {
	return func(v any) bool {
		msg, ok := v.(indexerTypes.IndexerMessageEnvelope)
		if !ok {
			return false
		}
		return msg.Action == action &&
			reflect.DeepEqual(msg.Data, *settings) &&
			reflect.DeepEqual(msg.IndexingConfig, settings.IndexingConfig(base.UID))
	}
}

func TestChunkStrings(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		size  int
		want  [][]string
	}{
		{
			name:  "empty input",
			items: nil,
			size:  2,
			want:  nil,
		},
		{
			name:  "single chunk under size",
			items: []string{"a", "b"},
			size:  5,
			want:  [][]string{{"a", "b"}},
		},
		{
			name:  "exact multiple of size",
			items: []string{"a", "b", "c", "d"},
			size:  2,
			want:  [][]string{{"a", "b"}, {"c", "d"}},
		},
		{
			name:  "boundary plus remainder",
			items: []string{"a", "b", "c"},
			size:  2,
			want:  [][]string{{"a", "b"}, {"c"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkStrings(tt.items, tt.size)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitResourceID(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantUID     string
		wantDocType string
		wantOK      bool
	}{
		{
			name:        "project doc",
			id:          "project:00000000-0000-0000-0000-000000000001",
			wantUID:     "00000000-0000-0000-0000-000000000001",
			wantDocType: "project",
			wantOK:      true,
		},
		{
			name:        "project_settings doc",
			id:          "project_settings:00000000-0000-0000-0000-000000000002",
			wantUID:     "00000000-0000-0000-0000-000000000002",
			wantDocType: "project_settings",
			wantOK:      true,
		},
		{
			name:   "unrecognized prefix",
			id:     "project_folder:00000000-0000-0000-0000-000000000003",
			wantOK: false,
		},
		{
			name:   "empty uid after prefix",
			id:     "project:",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, docType, ok := splitResourceID(tt.id)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantUID, uid)
				assert.Equal(t, tt.wantDocType, docType)
			}
		})
	}
}

// roundTripFunc lets a test stand up an *opensearchgo.Client backed by a
// canned HTTP response instead of a live OpenSearch cluster.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newFakeOpenSearchClient(t *testing.T, hitIDs []string) *opensearchgo.Client {
	t.Helper()

	body := `{"hits":{"hits":[`
	for i, id := range hitIDs {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"_id":%q}`, id)
	}
	body += `]}}`

	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})

	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: transport,
	})
	require.NoError(t, err)
	return client
}

// newRecordingOpenSearchClient behaves like newFakeOpenSearchClient but also
// appends each request body to requests, so a test can assert exactly which
// project UIDs were (or were not) queried against the resources index.
func newRecordingOpenSearchClient(t *testing.T, hitIDs []string, requests *[]string) *opensearchgo.Client {
	t.Helper()

	body := `{"hits":{"hits":[`
	for i, id := range hitIDs {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"_id":%q}`, id)
	}
	body += `]}}`

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		*requests = append(*requests, string(raw))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})

	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: transport,
	})
	require.NoError(t, err)
	return client
}

// captureSlog redirects the package-level slog default to a buffer for the
// duration of the test, restoring the prior default on cleanup.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func newFakeOpenSearchClientWithBody(t *testing.T, body string) *opensearchgo.Client {
	t.Helper()

	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})

	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{"http://opensearch.invalid"},
		Transport: transport,
	})
	require.NoError(t, err)
	return client
}

func TestReindexProjectsRunner_queryExistingIDs(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantIDs map[string]struct{}
		wantErr bool
	}{
		{
			name:    "clean result",
			body:    `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[{"_id":"project:1"}]}}`,
			wantIDs: map[string]struct{}{"project:1": {}},
		},
		{
			name:    "timed out",
			body:    `{"timed_out":true,"_shards":{"failed":0},"hits":{"hits":[]}}`,
			wantErr: true,
		},
		{
			name:    "shard failure",
			body:    `{"timed_out":false,"_shards":{"failed":1},"hits":{"hits":[]}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &reindexProjectsRunner{openSearch: newFakeOpenSearchClientWithBody(t, tt.body)}
			got, err := r.queryExistingIDs(context.Background(), []string{"project:1"})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIDs, got)
		})
	}
}

func TestReindexProjectsRunner_diffOpenSearch(t *testing.T) {
	tests := []struct {
		name   string
		bases  []*models.ProjectBase
		hitIDs []string
		want   map[string]osMissing
	}{
		{
			name: "both docs present",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000001"},
			},
			hitIDs: []string{
				"project:00000000-0000-0000-0000-000000000001",
				"project_settings:00000000-0000-0000-0000-000000000001",
			},
			want: map[string]osMissing{
				"00000000-0000-0000-0000-000000000001": {project: false, projectSettings: false},
			},
		},
		{
			name: "both docs missing",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000002"},
			},
			hitIDs: nil,
			want: map[string]osMissing{
				"00000000-0000-0000-0000-000000000002": {project: true, projectSettings: true},
			},
		},
		{
			name: "only settings doc missing",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000003"},
			},
			hitIDs: []string{
				"project:00000000-0000-0000-0000-000000000003",
			},
			want: map[string]osMissing{
				"00000000-0000-0000-0000-000000000003": {project: false, projectSettings: true},
			},
		},
		{
			name: "only project doc missing across multiple projects",
			bases: []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000004"},
				{UID: "00000000-0000-0000-0000-000000000005"},
			},
			hitIDs: []string{
				"project_settings:00000000-0000-0000-0000-000000000004",
				"project:00000000-0000-0000-0000-000000000005",
				"project_settings:00000000-0000-0000-0000-000000000005",
			},
			want: map[string]osMissing{
				"00000000-0000-0000-0000-000000000004": {project: true, projectSettings: false},
				"00000000-0000-0000-0000-000000000005": {project: false, projectSettings: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &reindexProjectsRunner{openSearch: newFakeOpenSearchClient(t, tt.hitIDs)}
			got, err := r.diffOpenSearch(context.Background(), tt.bases)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReindexProjectsRunner_reindexProject(t *testing.T) {
	base := &models.ProjectBase{UID: "00000000-0000-0000-0000-000000000010", Public: true}
	settings := &models.ProjectSettings{UID: "00000000-0000-0000-0000-000000000010"}

	tests := []struct {
		name          string
		missing       osMissing
		all           bool
		includeAccess bool
		setupMock     func(*domainmocks.MockMessageBuilder)
		getSettings   bool
		settingsErr   error
		wantErr       bool
	}{
		{
			name:    "missing project only",
			missing: osMissing{project: true},
			setupMock: func(m *domainmocks.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionCreated, base)), true).
					Return(nil).Once()
			},
		},
		{
			name:        "missing settings only",
			missing:     osMissing{projectSettings: true},
			getSettings: true,
			setupMock: func(m *domainmocks.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSettingsSubject,
					mock.MatchedBy(settingsEnvelopeMatcher(indexerConstants.ActionCreated, base, settings)), true).
					Return(nil).Once()
			},
		},
		{
			name:          "missing both plus access",
			missing:       osMissing{project: true, projectSettings: true},
			includeAccess: true,
			getSettings:   true,
			setupMock: func(m *domainmocks.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionCreated, base)), true).
					Return(nil).Once()
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSettingsSubject,
					mock.MatchedBy(settingsEnvelopeMatcher(indexerConstants.ActionCreated, base, settings)), true).
					Return(nil).Once()
				m.On("PublishAccessMessage", mock.Anything, mock.Anything, mock.Anything).
					Return(nil).Once()
			},
		},
		{
			name:        "all mode uses ActionUpdated",
			missing:     osMissing{project: true, projectSettings: true},
			all:         true,
			getSettings: true,
			setupMock: func(m *domainmocks.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionUpdated, base)), true).
					Return(nil).Once()
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSettingsSubject,
					mock.MatchedBy(settingsEnvelopeMatcher(indexerConstants.ActionUpdated, base, settings)), true).
					Return(nil).Once()
			},
		},
		{
			name:        "settings read failure still publishes an independently missing project doc",
			missing:     osMissing{project: true, projectSettings: true},
			getSettings: true,
			settingsErr: fmt.Errorf("settings kv record not found"),
			setupMock: func(m *domainmocks.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionCreated, base)), true).
					Return(nil).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &domainmocks.MockMessageBuilder{}
			tt.setupMock(publisher)

			r := &reindexProjectsRunner{
				repo: &fakeProjectRecordRepo{
					settingsByUID: map[string]*models.ProjectSettings{base.UID: settings},
					settingsErr:   tt.settingsErr,
				},
				publisher:     publisher,
				all:           tt.all,
				includeAccess: tt.includeAccess,
			}
			err := r.reindexProject(context.Background(), base, tt.missing)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			publisher.AssertExpectations(t)
		})
	}
}

// fakeProjectRecordRepo implements projectRecordRepo over an in-memory map,
// so reindexProject's envelope construction can be tested without a live
// NATS KV connection.
type fakeProjectRecordRepo struct {
	bases         []*models.ProjectBase
	baseByUID     map[string]*models.ProjectBase
	settingsByUID map[string]*models.ProjectSettings
	listErr       error
	baseErr       error
	settingsErr   error
}

func (f *fakeProjectRecordRepo) GetProjectBase(_ context.Context, projectUID string) (*models.ProjectBase, error) {
	if f.baseErr != nil {
		return nil, f.baseErr
	}
	base, ok := f.baseByUID[projectUID]
	if !ok {
		return nil, fmt.Errorf("project base %q not found", projectUID)
	}
	return base, nil
}

func (f *fakeProjectRecordRepo) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.bases, nil
}

func (f *fakeProjectRecordRepo) GetProjectSettings(_ context.Context, projectUID string) (*models.ProjectSettings, error) {
	if f.settingsErr != nil {
		return nil, f.settingsErr
	}
	return f.settingsByUID[projectUID], nil
}

// publishedProjectUIDs extracts the project UIDs the publisher was called
// with for constants.IndexProjectSubject or constants.IndexProjectSettingsSubject,
// so tests can assert exactly which projects were reindexed rather than just
// how many, and a settings-only publish for an excluded project isn't missed.
func publishedProjectUIDs(t *testing.T, publisher *domainmocks.MockMessageBuilder) map[string]bool {
	t.Helper()
	uids := map[string]bool{}
	for _, call := range publisher.Calls {
		if call.Method != "SendIndexerMessage" {
			continue
		}
		subject := call.Arguments.String(1)
		msg, ok := call.Arguments.Get(2).(indexerTypes.IndexerMessageEnvelope)
		require.True(t, ok)
		switch subject {
		case constants.IndexProjectSubject:
			base, ok := msg.Data.(models.ProjectBase)
			require.True(t, ok)
			uids[base.UID] = true
		case constants.IndexProjectSettingsSubject:
			settings, ok := msg.Data.(models.ProjectSettings)
			require.True(t, ok)
			uids[settings.UID] = true
		}
	}
	return uids
}

// publishedFGAUIDs extracts the project UIDs the publisher's
// PublishAccessMessage was called with, so tests can assert exactly which
// projects had their FGA access republished.
func publishedFGAUIDs(t *testing.T, publisher *domainmocks.MockMessageBuilder) map[string]bool {
	t.Helper()
	uids := map[string]bool{}
	for _, call := range publisher.Calls {
		if call.Method != "PublishAccessMessage" {
			continue
		}
		msg, ok := call.Arguments.Get(2).(fgatypes.GenericFGAMessage)
		require.True(t, ok)
		data, ok := msg.Data.(fgatypes.GenericAccessData)
		require.True(t, ok)
		uids[data.UID] = true
	}
	return uids
}

func TestReindexProjectsRunner_run(t *testing.T) {
	newBase := func(uid, slug string) *models.ProjectBase {
		return &models.ProjectBase{UID: uid, Slug: slug, Name: slug}
	}
	newSettings := func(uid string) *models.ProjectSettings {
		return &models.ProjectSettings{UID: uid}
	}

	const (
		alphaUID = "00000000-0000-0000-0000-000000000001"
		betaUID  = "00000000-0000-0000-0000-000000000002"
		rootUID  = "00000000-0000-0000-0000-000000000003"
	)

	tests := []struct {
		name             string
		projectUID       string
		bases            []*models.ProjectBase
		baseByUID        map[string]*models.ProjectBase
		listErr          error
		baseErr          error
		settingsErr      error
		all              bool
		dryRun           bool
		includeAccess    bool
		openSearchHitIDs []string
		wantErr          string
		wantUIDs         map[string]bool
		wantTotal        int
		wantUpdated      int
		wantFailed       int
		wantFGAUIDs      map[string]bool
		wantQueriedHas   []string
		wantQueriedNot   []string
	}{
		{
			name: "full scan excludes ROOT",
			all:  true,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "alpha-project"),
				newBase(betaUID, "beta-project"),
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:    map[string]bool{alphaUID: true, betaUID: true},
			wantTotal:   2,
			wantUpdated: 2,
		},
		{
			// Mirrors the only reachable explicit-UID state: reindex_projects.go
			// rejects --all combined with --project-uid, so the supported path
			// always goes through the OpenSearch diff below (all: false).
			name:       "explicit project-uid on the ROOT record still reindexes",
			projectUID: rootUID,
			baseByUID: map[string]*models.ProjectBase{
				rootUID: newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:       map[string]bool{rootUID: true},
			wantTotal:      1,
			wantUpdated:    1,
			wantQueriedHas: []string{"project:" + rootUID},
		},
		{
			name: "lowercase root is not excluded",
			all:  true,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "root"),
			},
			wantUIDs:    map[string]bool{alphaUID: true},
			wantTotal:   1,
			wantUpdated: 1,
		},
		{
			name: "no ROOT record present",
			all:  true,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "alpha-project"),
				newBase(betaUID, "beta-project"),
			},
			wantUIDs:    map[string]bool{alphaUID: true, betaUID: true},
			wantTotal:   2,
			wantUpdated: 2,
		},
		{
			name:          "default diff scan with include-access never repairs ROOT FGA",
			all:           false,
			includeAccess: true,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "alpha-project"),
				newBase(betaUID, "beta-project"),
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:    map[string]bool{alphaUID: true, betaUID: true},
			wantTotal:   2,
			wantUpdated: 2,
			wantFGAUIDs: map[string]bool{alphaUID: true, betaUID: true},
		},
		{
			name: "default diff scan never queries ROOT against OpenSearch",
			all:  false,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "alpha-project"),
				newBase(betaUID, "beta-project"),
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:       map[string]bool{alphaUID: true, betaUID: true},
			wantTotal:      2,
			wantUpdated:    2,
			wantQueriedHas: []string{"project:" + alphaUID, "project:" + betaUID},
			wantQueriedNot: []string{rootUID},
		},
		{
			name:          "all scan with include-access repairs ROOT FGA without indexing it",
			all:           true,
			includeAccess: true,
			bases: []*models.ProjectBase{
				newBase(alphaUID, "alpha-project"),
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:    map[string]bool{alphaUID: true},
			wantTotal:   2,
			wantUpdated: 2,
			wantFGAUIDs: map[string]bool{alphaUID: true, rootUID: true},
		},
		{
			name:          "failed ROOT access repair is recorded, not returned",
			all:           true,
			includeAccess: true,
			settingsErr:   fmt.Errorf("settings kv record not found"),
			bases: []*models.ProjectBase{
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:   map[string]bool{},
			wantTotal:  1,
			wantFailed: 1,
		},
		{
			// reindexRootAccess's dry-run branch returns before calling
			// GetProjectSettings, so a settingsErr here would surface as
			// stats.Failed if the dry-run guard regressed and let a real
			// settings lookup or FGA publish slip through during a preview.
			name:          "all --include-access dry-run skips ROOT settings lookup and FGA publish",
			all:           true,
			includeAccess: true,
			dryRun:        true,
			settingsErr:   fmt.Errorf("settings kv record not found"),
			bases: []*models.ProjectBase{
				newBase(rootUID, rootProjectSlug),
			},
			wantUIDs:    map[string]bool{},
			wantTotal:   1,
			wantUpdated: 1,
			wantFGAUIDs: map[string]bool{},
		},
		{
			name:    "ListAllProjectsBase error propagates",
			all:     true,
			listErr: fmt.Errorf("kv unavailable"),
			wantErr: "list project bases",
		},
		{
			name:       "GetProjectBase error propagates",
			projectUID: alphaUID,
			all:        true,
			baseErr:    fmt.Errorf("kv unavailable"),
			wantErr:    "get project base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsByUID := map[string]*models.ProjectSettings{
				alphaUID: newSettings(alphaUID),
				betaUID:  newSettings(betaUID),
				rootUID:  newSettings(rootUID),
			}
			repo := &fakeProjectRecordRepo{
				bases:         tt.bases,
				baseByUID:     tt.baseByUID,
				settingsByUID: settingsByUID,
				listErr:       tt.listErr,
				baseErr:       tt.baseErr,
				settingsErr:   tt.settingsErr,
			}
			publisher := &domainmocks.MockMessageBuilder{}
			publisher.On("SendIndexerMessage", mock.Anything, mock.Anything, mock.Anything, true).Return(nil)
			publisher.On("PublishAccessMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			var requests []string
			var osClient *opensearchgo.Client
			if !tt.all {
				osClient = newRecordingOpenSearchClient(t, tt.openSearchHitIDs, &requests)
			}

			r := &reindexProjectsRunner{
				repo:          repo,
				openSearch:    osClient,
				publisher:     publisher,
				all:           tt.all,
				dryRun:        tt.dryRun,
				includeAccess: tt.includeAccess,
				concurrency:   1,
				stats:         commands.NewStats(),
			}

			err := r.run(context.Background(), tt.projectUID)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUIDs, publishedProjectUIDs(t, publisher))
			assert.Equal(t, tt.wantTotal, r.stats.Total)
			assert.Equal(t, tt.wantUpdated, r.stats.Updated)
			assert.Equal(t, tt.wantFailed, r.stats.Failed)
			if tt.wantFGAUIDs != nil {
				assert.Equal(t, tt.wantFGAUIDs, publishedFGAUIDs(t, publisher))
			}
			combined := strings.Join(requests, "\n")
			for _, id := range tt.wantQueriedHas {
				assert.Contains(t, combined, id)
			}
			for _, id := range tt.wantQueriedNot {
				assert.NotContains(t, combined, id)
			}
		})
	}

	// The 1000-unit progress boundary can't be crossed by the ROOT-repair and
	// skip paths in the same run: ROOT-repair only fires under --all, but
	// --all marks every base missing (bypassing the OpenSearch diff), so the
	// skip branch can never trigger at the same time. These two cases instead
	// cross the boundary once via each path independently.
	t.Run("progress boundary logs processed and total at 1000", func(t *testing.T) {
		t.Run("via ROOT access repair under --all --include-access", func(t *testing.T) {
			settingsByUID := map[string]*models.ProjectSettings{rootUID: newSettings(rootUID)}
			bases := make([]*models.ProjectBase, 0, 999)
			for i := 0; i < 999; i++ {
				uid := fmt.Sprintf("00000000-0000-0000-0001-%012d", i+1)
				bases = append(bases, newBase(uid, fmt.Sprintf("project-%d", i)))
				settingsByUID[uid] = newSettings(uid)
			}
			bases = append(bases, newBase(rootUID, rootProjectSlug))

			repo := &fakeProjectRecordRepo{bases: bases, settingsByUID: settingsByUID}
			publisher := &domainmocks.MockMessageBuilder{}
			publisher.On("SendIndexerMessage", mock.Anything, mock.Anything, mock.Anything, true).Return(nil)
			publisher.On("PublishAccessMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			r := &reindexProjectsRunner{
				repo:          repo,
				publisher:     publisher,
				all:           true,
				includeAccess: true,
				concurrency:   1,
				stats:         commands.NewStats(),
			}

			logs := captureSlog(t)
			require.NoError(t, r.run(context.Background(), ""))
			assert.Equal(t, 1000, r.stats.Total)

			out := logs.String()
			assert.Contains(t, out, `"msg":"reindex-projects progress"`)
			assert.Contains(t, out, `"processed":1000`)
			assert.Contains(t, out, `"total":1000`)
		})

		t.Run("via a skipped already-indexed project on a default diff scan", func(t *testing.T) {
			settingsByUID := make(map[string]*models.ProjectSettings, 1000)
			bases := make([]*models.ProjectBase, 0, 1000)
			hitIDs := make([]string, 0, 2000)
			for i := 0; i < 1000; i++ {
				uid := fmt.Sprintf("00000000-0000-0000-0002-%012d", i+1)
				bases = append(bases, newBase(uid, fmt.Sprintf("project-%d", i)))
				settingsByUID[uid] = newSettings(uid)
				hitIDs = append(hitIDs, "project:"+uid, "project_settings:"+uid)
			}

			repo := &fakeProjectRecordRepo{bases: bases, settingsByUID: settingsByUID}
			publisher := &domainmocks.MockMessageBuilder{}
			publisher.On("SendIndexerMessage", mock.Anything, mock.Anything, mock.Anything, true).Return(nil)
			publisher.On("PublishAccessMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			var requests []string
			osClient := newRecordingOpenSearchClient(t, hitIDs, &requests)

			r := &reindexProjectsRunner{
				repo:        repo,
				openSearch:  osClient,
				publisher:   publisher,
				concurrency: 1,
				stats:       commands.NewStats(),
			}

			logs := captureSlog(t)
			require.NoError(t, r.run(context.Background(), ""))
			assert.Equal(t, 1000, r.stats.Total)
			assert.Equal(t, 1000, r.stats.Skipped)

			out := logs.String()
			assert.Contains(t, out, `"msg":"reindex-projects progress"`)
			assert.Contains(t, out, `"processed":1000`)
			assert.Contains(t, out, `"total":1000`)
		})
	})
}

func TestReindexProjectsSubcommand_flagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "all with project-uid is rejected",
			args:    []string{"--all", "--project-uid", "00000000-0000-0000-0000-000000000001"},
			wantErr: "--all and --project-uid are mutually exclusive",
		},
		{
			name:    "unexpected positional argument is rejected",
			args:    []string{"unexpected"},
			wantErr: "unexpected arguments: unexpected",
		},
		{
			name:    "zero concurrency is rejected",
			args:    []string{"--concurrency", "0"},
			wantErr: "concurrency must be at least 1",
		},
		{
			name:    "negative concurrency is rejected",
			args:    []string{"--concurrency", "-1"},
			wantErr: "concurrency must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &reindexProjectsSubcommand{}
			err := s.Run(context.Background(), commands.RunContext{Args: tt.args})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
