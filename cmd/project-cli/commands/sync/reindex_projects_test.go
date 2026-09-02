// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
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
		includeAccess bool
		setupMock     func(*domain.MockMessageBuilder)
		getSettings   bool
	}{
		{
			name:    "missing project only",
			missing: osMissing{project: true},
			setupMock: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionCreated, base)), false).
					Return(nil).Once()
			},
		},
		{
			name:        "missing settings only",
			missing:     osMissing{projectSettings: true},
			getSettings: true,
			setupMock: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSettingsSubject,
					mock.MatchedBy(settingsEnvelopeMatcher(indexerConstants.ActionCreated, base, settings)), false).
					Return(nil).Once()
			},
		},
		{
			name:          "missing both plus access",
			missing:       osMissing{project: true, projectSettings: true},
			includeAccess: true,
			getSettings:   true,
			setupMock: func(m *domain.MockMessageBuilder) {
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSubject,
					mock.MatchedBy(projectEnvelopeMatcher(indexerConstants.ActionCreated, base)), false).
					Return(nil).Once()
				m.On("SendIndexerMessage", mock.Anything, constants.IndexProjectSettingsSubject,
					mock.MatchedBy(settingsEnvelopeMatcher(indexerConstants.ActionCreated, base, settings)), false).
					Return(nil).Once()
				m.On("PublishAccessMessage", mock.Anything, mock.Anything, mock.Anything).
					Return(nil).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &domain.MockMessageBuilder{}
			tt.setupMock(publisher)

			r := &reindexProjectsRunner{
				repo:          &fakeProjectRecordRepo{settingsByUID: map[string]*models.ProjectSettings{base.UID: settings}},
				publisher:     publisher,
				includeAccess: tt.includeAccess,
			}
			err := r.reindexProject(context.Background(), base, tt.missing)
			require.NoError(t, err)
			publisher.AssertExpectations(t)
		})
	}
}

// fakeProjectRecordRepo implements projectRecordRepo over an in-memory map,
// so reindexProject's envelope construction can be tested without a live
// NATS KV connection.
type fakeProjectRecordRepo struct {
	settingsByUID map[string]*models.ProjectSettings
}

func (f *fakeProjectRecordRepo) GetProjectBase(context.Context, string) (*models.ProjectBase, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeProjectRecordRepo) ListAllProjectsBase(context.Context) ([]*models.ProjectBase, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeProjectRecordRepo) GetProjectSettings(_ context.Context, projectUID string) (*models.ProjectSettings, error) {
	return f.settingsByUID[projectUID], nil
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
