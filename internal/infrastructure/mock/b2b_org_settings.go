// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import (
	"context"
	"sync"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// MockB2BOrgSettings is an in-memory implementation of port.B2BOrgSettingsReader
// and port.B2BOrgSettingsWriter. It supports seeding a fixed settings value and
// revision for read tests, and records UpdateSettings calls for assertion in write tests.
type MockB2BOrgSettings struct {
	mu       sync.RWMutex
	settings map[string]*model.B2BOrgSettings
	revision map[string]uint64
	putErr   error
	listErr  error
}

// NewMockB2BOrgSettings returns an empty, ready-to-use mock.
func NewMockB2BOrgSettings() *MockB2BOrgSettings {
	return &MockB2BOrgSettings{
		settings: make(map[string]*model.B2BOrgSettings),
		revision: make(map[string]uint64),
	}
}

// Seed pre-populates the mock with a settings value and revision for the given orgUID.
func (m *MockB2BOrgSettings) Seed(orgUID string, s *model.B2BOrgSettings, rev uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[orgUID] = s
	m.revision[orgUID] = rev
}

// SetPutError configures the mock to return err on the next UpdateSettings call.
func (m *MockB2BOrgSettings) SetPutError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putErr = err
}

// SetListError configures the mock to return err on the next ListSettingsOrgUIDs call.
func (m *MockB2BOrgSettings) SetListError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listErr = err
}

// ListSettingsOrgUIDs returns all seeded org UIDs.
func (m *MockB2BOrgSettings) ListSettingsOrgUIDs(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		err := m.listErr
		m.listErr = nil
		return nil, err
	}
	uids := make([]string, 0, len(m.settings))
	for uid := range m.settings {
		uids = append(uids, uid)
	}
	return uids, nil
}

// GetSettings returns the seeded settings for orgUID, or (nil, 0, nil) when absent.
func (m *MockB2BOrgSettings) GetSettings(_ context.Context, orgUID string) (*model.B2BOrgSettings, uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.settings[orgUID]
	if !ok {
		return nil, 0, nil
	}
	return s, m.revision[orgUID], nil
}

// UpdateSettings stores settings for orgUID, mirroring production NATS semantics:
// revision == 0 → exclusive create (Conflict if already exists);
// revision > 0  → optimistic-lock update (Conflict if revision doesn't match).
func (m *MockB2BOrgSettings) UpdateSettings(_ context.Context, s *model.B2BOrgSettings, revision uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		err := m.putErr
		m.putErr = nil
		return err
	}
	orgUID := s.UID
	if revision == 0 {
		if _, exists := m.settings[orgUID]; exists {
			return errors.NewConflict("org settings were created concurrently, please retry")
		}
	} else {
		if stored, ok := m.revision[orgUID]; !ok || stored != revision {
			return errors.NewConflict("stale revision")
		}
	}
	m.settings[orgUID] = s
	m.revision[orgUID] = revision + 1
	return nil
}
