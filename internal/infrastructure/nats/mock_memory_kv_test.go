// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

// memKV is an in-memory bucket with the revision semantics the repository relies on: a revision that grows with every
// write to the bucket, creation that fails on an existing key, updates and deletes that fail on a stale revision, and
// purges and deletes that honour jetstream.LastRevision. Methods the repository does not use panic through the
// embedded interface.
type memKV struct {
	INatsKeyValue
	mu       sync.Mutex
	entries  map[string]*MockKeyValueEntry
	revision uint64
}

func newMemKV() *memKV {
	return &memKV{entries: map[string]*MockKeyValueEntry{}}
}

func (kv *memKV) store(key string, value []byte) uint64 {
	kv.revision++
	kv.entries[key] = NewMockKeyValueEntry(append([]byte(nil), value...), kv.revision)
	return kv.revision
}

func (kv *memKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists {
		return nil, jetstream.ErrKeyNotFound
	}
	return entry, nil
}

func (kv *memKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if _, exists := kv.entries[key]; exists {
		return 0, jetstream.ErrKeyExists
	}
	return kv.store(key, value), nil
}

func (kv *memKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	return kv.store(key, value), nil
}

func (kv *memKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists || entry.Revision() != revision {
		return 0, fmt.Errorf("nats: wrong last sequence: %d", revision)
	}
	return kv.store(key, value), nil
}

func (kv *memKV) Delete(_ context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	return kv.remove(key, opts)
}

func (kv *memKV) Purge(_ context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	return kv.remove(key, opts)
}

func (kv *memKV) remove(key string, opts []jetstream.KVDeleteOpt) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	entry, exists := kv.entries[key]
	if !exists {
		return jetstream.ErrKeyNotFound
	}
	for _, opt := range opts {
		// LastRevision is opaque: apply it to a fresh options struct and read the revision back, as forwardedRevision does.
		fn := reflect.ValueOf(opt)
		target := reflect.New(fn.Type().In(0).Elem())
		fn.Call([]reflect.Value{target})
		// A revision of zero is no guard at all, as in nats.go.
		if want := target.Elem().FieldByName("revision").Uint(); want != 0 && want != entry.Revision() {
			return fmt.Errorf("nats: wrong last sequence: %d", entry.Revision())
		}
	}
	delete(kv.entries, key)
	return nil
}
