// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
)

// INatsConn is a NATS connection interface needed for the [ProjectsService].
type INatsConn interface {
	IsConnected() bool
	Publish(subj string, data []byte) error
}

// MockNATSConn is a mock implementation of the [INatsConn] interface.
type MockNATSConn struct {
	mock.Mock
}

// IsConnected is a mock method for the [INatsConn] interface.
func (m *MockNATSConn) IsConnected() bool {
	args := m.Called()
	return args.Bool(0)
}

// Publish is a mock method for the [INatsConn] interface.
func (m *MockNATSConn) Publish(subj string, data []byte) error {
	args := m.Called(subj, data)
	return args.Error(0)
}

// INatsMsg is an interface for [nats.Msg] that allows for mocking.
type INatsMsg interface {
	Respond(data []byte) error
	Data() []byte
	Subject() string
}

// NatsMsg is a wrapper around [nats.Msg] that implements [INatsMsg].
type NatsMsg struct {
	*nats.Msg
}

// Respond implements [INatsMsg.Respond].
func (m *NatsMsg) Respond(data []byte) error {
	return m.Msg.Respond(data)
}

// Data implements [INatsMsg.Data].
func (m *NatsMsg) Data() []byte {
	return m.Msg.Data
}

// Subject implements [INatsMsg.Subject].
func (m *NatsMsg) Subject() string {
	return m.Msg.Subject
}

// Ensure NatsMsg implements domain.Message interface
var _ domain.Message = (*NatsMsg)(nil)

type MockNatsMsg struct {
	mock.Mock
	data    []byte
	subject string
}

func (m *MockNatsMsg) Respond(data []byte) error {
	args := m.Called(data)
	return args.Error(0)
}

func (m *MockNatsMsg) Data() []byte {
	return m.data
}

func (m *MockNatsMsg) Subject() string {
	return m.subject
}

// INatsKeyValue is a NATS KV interface needed for the [ProjectsService].
type INatsKeyValue interface {
	ListKeys(context.Context, ...jetstream.WatchOpt) (jetstream.KeyLister, error)
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(context.Context, string, []byte) (uint64, error)
	Update(context.Context, string, []byte, uint64) (uint64, error)
	Delete(context.Context, string, ...jetstream.KVDeleteOpt) error
}

// MockKeyValue is a mock implementation of the [INatsKeyValue] interface.
type MockKeyValue struct {
	mock.Mock
}

// Put is a mock method for the [INatsKeyValue] interface.
func (m *MockKeyValue) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	args := m.Called(ctx, key, value)
	return args.Get(0).(uint64), args.Error(1)
}

// Get is a mock method for the [INatsKeyValue] interface.
func (m *MockKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(jetstream.KeyValueEntry), args.Error(1)
}

// Update is a mock method for the [INatsKeyValue] interface.
func (m *MockKeyValue) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	args := m.Called(ctx, key, value, revision)
	return args.Get(0).(uint64), args.Error(1)
}

// Delete is a mock method for the [INatsKeyValue] interface.
func (m *MockKeyValue) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	args := m.Called(ctx, key, opts)
	return args.Error(0)
}

// ListKeys is a mock method for the [INatsKeyValue] interface.
func (m *MockKeyValue) ListKeys(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	args := m.Called(ctx)
	return args.Get(0).(jetstream.KeyLister), args.Error(1)
}

// MockKeyLister is a mock implementation of the [jetstream.KeyLister] interface.
type MockKeyLister struct {
	mock.Mock
	keys []string
}

func NewMockKeyLister(keys []string) *MockKeyLister {
	return &MockKeyLister{
		keys: keys,
	}
}

// Keys is a mock method for the [jetstream.KeyLister] interface.
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

// Stop is a mock method for the [jetstream.KeyLister] interface.
func (m *MockKeyLister) Stop() error {
	args := m.Called()
	return args.Error(0)
}

// MockKeyValueEntry is a mock implementation of the [jetstream.KeyValueEntry] interface.
type MockKeyValueEntry struct {
	mock.Mock
	value    []byte
	revision uint64
}

func NewMockKeyValueEntry(value []byte, revision uint64) *MockKeyValueEntry {
	return &MockKeyValueEntry{
		value:    value,
		revision: revision,
	}
}

// Key is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Key() string {
	args := m.Called()
	return args.String(0)
}

// Value is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Value() []byte {
	return m.value
}

// Revision is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Revision() uint64 {
	return m.revision
}

// Created is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Created() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}

// Delta is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Delta() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

// Operation is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Operation() jetstream.KeyValueOp {
	args := m.Called()
	return args.Get(0).(jetstream.KeyValueOp)
}

// Bucket is a mock method for the [jetstream.KeyValueEntry] interface.
func (m *MockKeyValueEntry) Bucket() string {
	args := m.Called()
	return args.String(0)
}
