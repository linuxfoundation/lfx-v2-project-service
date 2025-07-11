// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"
)

// Mock NATS connection and KV store for testing
type MockNATSConn struct {
	mock.Mock
}

func (m *MockNATSConn) IsConnected() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockNATSConn) Publish(subj string, data []byte) error {
	args := m.Called(subj, data)
	return args.Error(0)
}

type MockKeyValue struct {
	mock.Mock
}

func (m *MockKeyValue) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	args := m.Called(ctx, key, value)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(jetstream.KeyValueEntry), args.Error(1)
}

func (m *MockKeyValue) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	args := m.Called(ctx, key, value, revision)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockKeyValue) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	args := m.Called(ctx, key, opts)
	return args.Error(0)
}

func (m *MockKeyValue) ListKeys(ctx context.Context, opts ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	args := m.Called(ctx)
	return args.Get(0).(jetstream.KeyLister), args.Error(1)
}

type MockKeyLister struct {
	mock.Mock
	keys []string
}

func (m *MockKeyLister) Keys() <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, key := range m.keys {
			ch <- key
		}
	}()
	return ch
}

func (m *MockKeyLister) Stop() error {
	args := m.Called()
	return args.Error(0)
}

type MockKeyValueEntry struct {
	mock.Mock
	value    []byte
	revision uint64
}

func (m *MockKeyValueEntry) Key() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockKeyValueEntry) Value() []byte {
	return m.value
}

func (m *MockKeyValueEntry) Revision() uint64 {
	return m.revision
}

func (m *MockKeyValueEntry) Created() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}

func (m *MockKeyValueEntry) Delta() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

func (m *MockKeyValueEntry) Operation() jetstream.KeyValueOp {
	args := m.Called()
	return args.Get(0).(jetstream.KeyValueOp)
}

func (m *MockKeyValueEntry) Bucket() string {
	args := m.Called()
	return args.String(0)
}

type MockJwtAuth struct {
	mock.Mock
}

func (m *MockJwtAuth) parsePrincipal(ctx context.Context, token string, logger *slog.Logger) (string, error) {
	args := m.Called(ctx, token, logger)
	return args.String(0), args.Error(1)
}
