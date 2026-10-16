// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	domainmocks "github.com/linuxfoundation/lfx-v2-project-service/internal/domain/mocks"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

func TestParseDocumentResourceType(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantFolders bool
		wantLinks   bool
		wantDocs    bool
		wantErr     bool
	}{
		{name: "empty means all", wantFolders: true, wantLinks: true, wantDocs: true},
		{name: "folder only", in: "folder", wantFolders: true},
		{name: "link only", in: "link", wantLinks: true},
		{name: "document only", in: "document", wantDocs: true},
		{name: "case and whitespace are normalised", in: "  Document ", wantDocs: true},
		{name: "invalid", in: "bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folders, links, docs, err := parseDocumentResourceType(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFolders, folders)
			assert.Equal(t, tt.wantLinks, links)
			assert.Equal(t, tt.wantDocs, docs)
		})
	}
}

func TestDocumentAuditUsersRunner_applyAuditUsers(t *testing.T) {
	aliceReader := func(t *testing.T) *domainmocks.MockUserReader {
		t.Helper()
		mockUser := &domainmocks.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
			Name: "Alice Example",
		}, nil)
		return mockUser
	}

	aliceAndBobReader := func(t *testing.T) *domainmocks.MockUserReader {
		t.Helper()
		mockUser := &domainmocks.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "bob").Return(&domain.UserMetadata{
			Name: "Bob Example",
		}, nil)
		return mockUser
	}

	tests := []struct {
		name        string
		runner      *documentAuditUsersRunner
		resource    freshAuditResource
		wantSkipped bool
		wantUpdated bool
		wantApplied bool
	}{
		{
			name:   "skips enriched records",
			runner: &documentAuditUsersRunner{stats: commands.NewStats()},
			resource: freshAuditResource{
				resourceType: "folder",
				uid:          "f1",
				projectUID:   "p1",
				createdBy:    &models.UserInfo{Username: "alice", Name: "Alice Example"},
			},
			wantSkipped: true,
		},
		{
			name: "preview counts updated",
			runner: &documentAuditUsersRunner{
				userReader: aliceReader(t),
				dryRun:     true,
				stats:      commands.NewStats(),
			},
			resource: freshAuditResource{
				resourceType: "link",
				uid:          "l1",
				projectUID:   "p1",
				createdBy:    &models.UserInfo{Username: "alice"},
			},
			wantUpdated: true,
		},
		{
			name: "writes enriched profile",
			runner: &documentAuditUsersRunner{
				userReader: aliceReader(t),
				dryRun:     false,
				stats:      commands.NewStats(),
			},
			resource: freshAuditResource{
				resourceType: "document",
				uid:          "d1",
				projectUID:   "p1",
				createdBy:    &models.UserInfo{Username: "alice"},
			},
			wantUpdated: true,
			wantApplied: true,
		},
		{
			name: "migrates distinct updated_by independently",
			runner: &documentAuditUsersRunner{
				userReader: aliceAndBobReader(t),
				dryRun:     false,
				stats:      commands.NewStats(),
			},
			resource: freshAuditResource{
				resourceType: "link",
				uid:          "l2",
				projectUID:   "p1",
				createdBy:    &models.UserInfo{Username: "alice", Name: "Alice Example"},
				updatedBy:    &models.UserInfo{Username: "bob"},
			},
			wantUpdated: true,
			wantApplied: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied := false
			err := tt.runner.applyAuditUsers(context.Background(), tt.resource, func(createdBy, updatedBy *models.UserInfo) error {
				applied = true
				if tt.resource.updatedBy != nil && models.AuditUserNeedsMigration(tt.resource.updatedBy) {
					require.NotNil(t, updatedBy)
					assert.Equal(t, "bob", updatedBy.Username)
					assert.Equal(t, "Bob Example", updatedBy.Name)
					return nil
				}
				require.NotNil(t, createdBy)
				assert.Equal(t, "Alice Example", createdBy.Name)
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantApplied, applied)
			if tt.wantSkipped {
				assert.Equal(t, 1, tt.runner.stats.Skipped)
			}
			if tt.wantUpdated {
				assert.Equal(t, 1, tt.runner.stats.Updated)
			}
			if mockUser, ok := tt.runner.userReader.(*domainmocks.MockUserReader); ok {
				mockUser.AssertExpectations(t)
			}
		})
	}
}

func TestDocumentAuditUsersRunner_throttle(t *testing.T) {
	t.Run("zero sleep returns immediately", func(t *testing.T) {
		runner := &documentAuditUsersRunner{sleep: 0}
		require.NoError(t, runner.throttle(context.Background()))
	})

	t.Run("waits for the configured duration", func(t *testing.T) {
		runner := &documentAuditUsersRunner{sleep: 10 * time.Millisecond}
		start := time.Now()
		require.NoError(t, runner.throttle(context.Background()))
		assert.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
	})

	t.Run("a cancelled context interrupts the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runner := &documentAuditUsersRunner{sleep: time.Minute}
		require.ErrorIs(t, runner.throttle(ctx), context.Canceled)
	})
}

func TestDocumentAuditUsersRunner_resolveAuditProfile(t *testing.T) {
	res := freshAuditResource{resourceType: "link", uid: "l1", projectUID: "p1"}

	t.Run("returns an enriched profile", func(t *testing.T) {
		reader := &domainmocks.MockUserReader{}
		reader.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{Name: "Alice Example"}, nil)
		r := &documentAuditUsersRunner{userReader: reader, stats: commands.NewStats()}

		profile, ok := r.resolveAuditProfile(context.Background(), res, "alice")

		require.True(t, ok)
		assert.Equal(t, "Alice Example", profile.Name)
		assert.Equal(t, 0, r.stats.Failed)
	})

	t.Run("blank username counts as a failure", func(t *testing.T) {
		r := &documentAuditUsersRunner{userReader: &domainmocks.MockUserReader{}, stats: commands.NewStats()}

		profile, ok := r.resolveAuditProfile(context.Background(), res, "  ")

		assert.False(t, ok)
		assert.Nil(t, profile)
		assert.Equal(t, 1, r.stats.Failed)
	})

	t.Run("a profile left without a name counts as a failure", func(t *testing.T) {
		reader := &domainmocks.MockUserReader{}
		reader.On("UserMetadataByPrincipal", mock.Anything, "ghost").Return(nil, errors.New("not found"))
		r := &documentAuditUsersRunner{userReader: reader, stats: commands.NewStats()}

		profile, ok := r.resolveAuditProfile(context.Background(), res, "ghost")

		assert.False(t, ok)
		assert.Nil(t, profile)
		assert.Equal(t, 1, r.stats.Failed)
	})
}

func TestDocumentAuditSleep(t *testing.T) {
	t.Run("returns nil after the duration", func(t *testing.T) {
		require.NoError(t, documentAuditSleep(context.Background(), time.Millisecond))
	})

	t.Run("returns the context error when cancelled first", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, documentAuditSleep(ctx, time.Minute), context.Canceled)
	})
}

// fakeAuditRepo is an in-memory documentAuditRepo that records writes and
// lets a test inject a failure for any operation.
type fakeAuditRepo struct {
	folders []*models.ProjectFolder
	links   []*models.ProjectLink
	docs    []*models.ProjectDocument

	listFoldersErr, listLinksErr, listDocsErr error
	getErr, updateErr                         error

	updatedFolders []*models.ProjectFolder
	updatedLinks   []*models.ProjectLink
	updatedDocs    []*models.ProjectDocument
	listedProject  string
}

func (f *fakeAuditRepo) ListFolders(_ context.Context, projectUID string) ([]*models.ProjectFolder, error) {
	f.listedProject = projectUID
	return f.folders, f.listFoldersErr
}

func (f *fakeAuditRepo) GetFolder(_ context.Context, _, folderUID string) (*models.ProjectFolder, uint64, error) {
	if f.getErr != nil {
		return nil, 0, f.getErr
	}
	for _, folder := range f.folders {
		if folder.UID == folderUID {
			clone := *folder
			return &clone, 7, nil
		}
	}
	return nil, 0, errors.New("folder not found")
}

func (f *fakeAuditRepo) UpdateFolder(_ context.Context, folder *models.ProjectFolder, revision uint64) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if revision != 7 {
		return errors.New("unexpected revision")
	}
	f.updatedFolders = append(f.updatedFolders, folder)
	return nil
}

func (f *fakeAuditRepo) ListAllLinks(_ context.Context, projectUID string) ([]*models.ProjectLink, error) {
	f.listedProject = projectUID
	return f.links, f.listLinksErr
}

func (f *fakeAuditRepo) GetLink(_ context.Context, _, linkUID string) (*models.ProjectLink, uint64, error) {
	if f.getErr != nil {
		return nil, 0, f.getErr
	}
	for _, link := range f.links {
		if link.UID == linkUID {
			clone := *link
			return &clone, 7, nil
		}
	}
	return nil, 0, errors.New("link not found")
}

func (f *fakeAuditRepo) UpdateLink(_ context.Context, link *models.ProjectLink, revision uint64) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if revision != 7 {
		return errors.New("unexpected revision")
	}
	f.updatedLinks = append(f.updatedLinks, link)
	return nil
}

func (f *fakeAuditRepo) ListAllDocuments(_ context.Context, projectUID string) ([]*models.ProjectDocument, error) {
	f.listedProject = projectUID
	return f.docs, f.listDocsErr
}

func (f *fakeAuditRepo) GetDocumentMetadata(_ context.Context, _, docUID string) (*models.ProjectDocument, uint64, error) {
	if f.getErr != nil {
		return nil, 0, f.getErr
	}
	for _, doc := range f.docs {
		if doc.UID == docUID {
			clone := *doc
			return &clone, 7, nil
		}
	}
	return nil, 0, errors.New("document not found")
}

func (f *fakeAuditRepo) UpdateDocumentMetadata(_ context.Context, doc *models.ProjectDocument, revision uint64) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if revision != 7 {
		return errors.New("unexpected revision")
	}
	f.updatedDocs = append(f.updatedDocs, doc)
	return nil
}

func TestDocumentAuditUsersRunner_run(t *testing.T) {
	const projectUID = "00000000-0000-0000-0000-000000000001"

	// Each resource was created by "alice", whose profile still needs backfilling.
	newRepo := func() *fakeAuditRepo {
		needsProfile := func() *models.UserInfo { return &models.UserInfo{Username: "alice"} }
		return &fakeAuditRepo{
			folders: []*models.ProjectFolder{{UID: "f1", ProjectUID: projectUID, Name: "Folder", CreatedBy: needsProfile()}},
			links:   []*models.ProjectLink{{UID: "l1", ProjectUID: projectUID, Name: "Link", CreatedBy: needsProfile()}},
			docs:    []*models.ProjectDocument{{UID: "d1", ProjectUID: projectUID, Name: "Doc", CreatedBy: needsProfile()}},
		}
	}

	subjects := []string{constants.IndexProjectFolderSubject, constants.IndexProjectLinkSubject, constants.IndexProjectDocumentSubject}

	tests := []struct {
		name        string
		mutateRepo  func(*fakeAuditRepo)
		reindexOnly bool
		dryRun      bool
		folders     bool
		links       bool
		docs        bool
		publishErr  error
		wantErr     string
		wantTotal   int
		wantUpdated int
		wantFailed  int
		wantWrites  int
		wantSent    int
	}{
		{
			name:    "apply backfills every resource type and publishes an index message for each",
			folders: true, links: true, docs: true,
			wantTotal: 3, wantUpdated: 3, wantWrites: 3, wantSent: 3,
		},
		{
			name:   "dry run counts drift but writes and publishes nothing",
			dryRun: true, folders: true, links: true, docs: true,
			wantTotal: 3, wantUpdated: 3,
		},
		{
			name:      "resource type filter limits the run",
			folders:   true,
			wantTotal: 1, wantUpdated: 1, wantWrites: 1, wantSent: 1,
		},
		{
			name:        "reindex only republishes without writing to KV",
			reindexOnly: true, folders: true, links: true, docs: true,
			wantTotal: 3, wantUpdated: 3, wantSent: 3,
		},
		{
			name:        "reindex only dry run publishes nothing",
			reindexOnly: true, dryRun: true, folders: true, links: true, docs: true,
			wantTotal: 3, wantUpdated: 3,
		},
		{
			name:        "reindex only counts a publish failure and keeps going",
			reindexOnly: true, folders: true, links: true, docs: true,
			publishErr: errors.New("nats down"),
			wantTotal:  3, wantFailed: 3, wantSent: 3,
		},
		{
			name:    "a publish failure after a successful write is counted as failed",
			folders: true, links: true, docs: true,
			publishErr: errors.New("nats down"),
			wantTotal:  3, wantFailed: 3, wantWrites: 3, wantSent: 3,
		},
		{
			name:       "a failed KV update is counted and nothing is published",
			mutateRepo: func(r *fakeAuditRepo) { r.updateErr = errors.New("revision conflict") },
			folders:    true, links: true, docs: true,
			wantTotal: 3, wantFailed: 3,
		},
		{
			name:       "a failed re-read is counted and the run continues",
			mutateRepo: func(r *fakeAuditRepo) { r.getErr = errors.New("kv unavailable") },
			folders:    true, links: true, docs: true,
			wantTotal: 3, wantFailed: 3,
		},
		{
			name:        "a failed re-read during reindex is counted",
			mutateRepo:  func(r *fakeAuditRepo) { r.getErr = errors.New("kv unavailable") },
			reindexOnly: true, folders: true, links: true, docs: true,
			wantTotal: 3, wantFailed: 3,
		},
		{
			name: "records that are already enriched are skipped",
			mutateRepo: func(r *fakeAuditRepo) {
				r.folders[0].CreatedBy = &models.UserInfo{Username: "alice", Name: "Alice Example"}
				r.links[0].CreatedBy = &models.UserInfo{Username: "alice", Name: "Alice Example"}
				r.docs[0].CreatedBy = &models.UserInfo{Username: "alice", Name: "Alice Example"}
			},
			folders: true, links: true, docs: true,
			wantTotal: 3,
		},
		{
			name:       "list folders failure aborts the run",
			mutateRepo: func(r *fakeAuditRepo) { r.listFoldersErr = errors.New("boom") },
			folders:    true,
			wantErr:    "list folders",
		},
		{
			name:       "list links failure aborts the run",
			mutateRepo: func(r *fakeAuditRepo) { r.listLinksErr = errors.New("boom") },
			links:      true,
			wantErr:    "list links",
		},
		{
			name:       "list documents failure aborts the run",
			mutateRepo: func(r *fakeAuditRepo) { r.listDocsErr = errors.New("boom") },
			docs:       true,
			wantErr:    "list documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepo()
			if tt.mutateRepo != nil {
				tt.mutateRepo(repo)
			}
			reader := &domainmocks.MockUserReader{}
			reader.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{Name: "Alice Example"}, nil)
			publisher := &domainmocks.MockMessageBuilder{}
			for _, subject := range subjects {
				publisher.On("SendIndexerMessage", mock.Anything, subject, mock.Anything).Return(tt.publishErr).Maybe()
			}

			r := &documentAuditUsersRunner{
				repo:        repo,
				userReader:  reader,
				publisher:   publisher,
				dryRun:      tt.dryRun,
				reindexOnly: tt.reindexOnly,
				stats:       commands.NewStats(),
			}

			err := r.run(context.Background(), projectUID, tt.folders, tt.links, tt.docs)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, projectUID, repo.listedProject)
			assert.Equal(t, tt.wantTotal, r.stats.Total)
			assert.Equal(t, tt.wantUpdated, r.stats.Updated)
			assert.Equal(t, tt.wantFailed, r.stats.Failed)
			assert.Equal(t, tt.wantWrites, len(repo.updatedFolders)+len(repo.updatedLinks)+len(repo.updatedDocs))
			assert.Len(t, publisher.Calls, tt.wantSent)
		})
	}

	t.Run("a backfill writes the resolved profile into the stored record", func(t *testing.T) {
		repo := newRepo()
		reader := &domainmocks.MockUserReader{}
		reader.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{Name: "Alice Example"}, nil)
		publisher := &domainmocks.MockMessageBuilder{}
		publisher.On("SendIndexerMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		r := &documentAuditUsersRunner{repo: repo, userReader: reader, publisher: publisher, stats: commands.NewStats()}

		require.NoError(t, r.run(context.Background(), projectUID, true, true, true))

		require.Len(t, repo.updatedFolders, 1)
		assert.Equal(t, "Alice Example", repo.updatedFolders[0].CreatedBy.Name)
		require.Len(t, repo.updatedLinks, 1)
		assert.Equal(t, "Alice Example", repo.updatedLinks[0].CreatedBy.Name)
		require.Len(t, repo.updatedDocs, 1)
		assert.Equal(t, "Alice Example", repo.updatedDocs[0].CreatedBy.Name)
	})
}

func TestDocumentAuditUsersSubcommand_Run(t *testing.T) {
	s := &documentAuditUsersSubcommand{}
	assert.Equal(t, "document-audit-users", s.Name())
	assert.NotEmpty(t, s.Help())

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "help is not an error", args: []string{"-h"}},
		{name: "unexpected positional argument is rejected", args: []string{"extra"}, wantErr: "unexpected arguments: extra"},
		{name: "invalid resource type is rejected", args: []string{"--resource-type=bad"}, wantErr: "invalid --resource-type"},
		{name: "unknown flag is rejected", args: []string{"--nope"}, wantErr: "flag provided but not defined"},
		{name: "a valid invocation proceeds to connect to NATS", args: []string{"--update", "--resource-type=link", "--sleep=1ms"}, wantErr: "failed to connect to NATS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Run(context.Background(), commands.RunContext{Args: tt.args, NATSConfig: unreachableNATS()})
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
