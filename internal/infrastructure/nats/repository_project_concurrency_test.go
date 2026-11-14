// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conditionalProjectKV models atomic KV writes for interleaving repository
// calls. Unlike fixed mock responses, a second claimant observes the first
// claimant's reservation. Unused interface methods intentionally panic.
type conditionalProjectKV struct {
	INatsKeyValue
	mu       sync.Mutex
	entries  map[string]*MockKeyValueEntry
	revision uint64
}

func (kv *conditionalProjectKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if _, exists := kv.entries[key]; exists {
		return 0, jetstream.ErrKeyExists
	}
	return kv.store(key, value), nil
}

func (kv *conditionalProjectKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	return kv.store(key, value), nil
}

func (kv *conditionalProjectKV) store(key string, value []byte) uint64 {
	kv.revision++
	kv.entries[key] = NewMockKeyValueEntry(append([]byte(nil), value...), kv.revision)
	return kv.revision
}

func (kv *conditionalProjectKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists {
		return nil, jetstream.ErrKeyNotFound
	}
	return entry, nil
}

func (kv *conditionalProjectKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists || entry.Revision() != revision {
		return 0, jetstream.ErrKeyExists
	}
	return kv.store(key, value), nil
}

func (kv *conditionalProjectKV) Delete(_ context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists {
		return jetstream.ErrKeyNotFound
	}
	// Apply LastRevision just as forwardedRevision does for fixed mock tests.
	for _, opt := range opts {
		fn := reflect.ValueOf(opt)
		target := reflect.New(fn.Type().In(0).Elem())
		fn.Call([]reflect.Value{target})
		if target.Elem().FieldByName("revision").Uint() != entry.Revision() {
			return jetstream.ErrKeyExists
		}
	}
	delete(kv.entries, key)
	return nil
}

func testConcurrentProjectCreates(t *testing.T) {
	for _, differentParents := range []bool{false, true} {
		t.Run(fmt.Sprintf("different_parents=%t", differentParents), func(t *testing.T) {
			projects := &conditionalProjectKV{entries: map[string]*MockKeyValueEntry{}}
			settings := &conditionalProjectKV{entries: map[string]*MockKeyValueEntry{}}
			repo := &NatsRepository{Projects: projects, ProjectSettings: settings}
			start := make(chan struct{})
			results := make(chan error, 2)
			bases := []*models.ProjectBase{
				{UID: "00000000-0000-0000-0000-000000000001", Slug: "shared-slug", ParentUID: "00000000-0000-0000-0000-000000000003"},
				{UID: "00000000-0000-0000-0000-000000000002", Slug: "shared-slug", ParentUID: "00000000-0000-0000-0000-000000000003", LogoURL: "https://example.com/logo.svg"},
			}
			if differentParents {
				bases[1].ParentUID = "00000000-0000-0000-0000-000000000004"
			}
			for _, base := range bases {
				go func() {
					<-start
					results <- repo.CreateProject(t.Context(), base, &models.ProjectSettings{UID: base.UID})
				}()
			}
			close(start)
			assertOneSlugWinner(t, results)
			winner, err := repo.GetProjectUIDFromSlug(t.Context(), "shared-slug")
			require.NoError(t, err)
			for _, base := range bases {
				_, baseErr := repo.GetProjectBase(t.Context(), base.UID)
				_, settingsErr := settings.Get(t.Context(), base.UID)
				if base.UID == winner {
					require.NoError(t, baseErr)
					require.NoError(t, settingsErr)
				} else {
					assert.ErrorIs(t, baseErr, domain.ErrProjectNotFound)
					assert.ErrorIs(t, settingsErr, jetstream.ErrKeyNotFound)
				}
			}
		})
	}
}

func assertOneSlugWinner(t *testing.T, results <-chan error) {
	t.Helper()
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	require.NoError(t, first)
	require.ErrorIs(t, second, domain.ErrProjectSlugExists)
}

func testConcurrentProjectRenames(t *testing.T) {
	projects := &conditionalProjectKV{entries: map[string]*MockKeyValueEntry{}}
	repo := &NatsRepository{Projects: projects}
	start := make(chan struct{})
	results := make(chan error, 2)
	bases := []*models.ProjectBase{
		{UID: "00000000-0000-0000-0000-000000000001", Slug: "alpha", ParentUID: "00000000-0000-0000-0000-000000000003"},
		{UID: "00000000-0000-0000-0000-000000000002", Slug: "beta", ParentUID: "00000000-0000-0000-0000-000000000004"},
	}
	for _, base := range bases {
		revision, err := projects.Put(t.Context(), base.UID, mustMarshal(t, base))
		require.NoError(t, err)
		_, err = projects.Put(t.Context(), "slug/"+base.Slug, []byte(base.UID))
		require.NoError(t, err)
		updated := *base
		updated.Slug = "shared-slug"
		go func() {
			<-start
			results <- repo.UpdateProjectBase(t.Context(), &updated, revision)
		}()
	}
	close(start)
	assertOneSlugWinner(t, results)
	winner, err := repo.GetProjectUIDFromSlug(t.Context(), "shared-slug")
	require.NoError(t, err)
	for _, base := range bases {
		stored, err := repo.GetProjectBase(t.Context(), base.UID)
		require.NoError(t, err)
		if base.UID == winner {
			assert.Equal(t, "shared-slug", stored.Slug)
			_, err = repo.GetProjectUIDFromSlug(t.Context(), base.Slug)
			assert.ErrorIs(t, err, domain.ErrProjectNotFound)
		} else {
			assert.Equal(t, base.Slug, stored.Slug)
			oldOwner, err := repo.GetProjectUIDFromSlug(t.Context(), base.Slug)
			require.NoError(t, err)
			assert.Equal(t, base.UID, oldOwner)
		}
	}
}
