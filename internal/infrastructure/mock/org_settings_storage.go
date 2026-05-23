// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package mock

import (
	"context"
	"sync"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// MockOrgSettingsStorage is an in-memory implementation of port.OrgSettingsStorage.
// It supports seeding a fixed settings value and revision for read tests, and
// records Put calls for assertion in write tests.
type MockOrgSettingsStorage struct {
	mu       sync.RWMutex
	settings map[string]*model.OrgSettings
	revision map[string]uint64
	putErr   error
}

// NewMockOrgSettingsStorage returns an empty, ready-to-use mock.
func NewMockOrgSettingsStorage() *MockOrgSettingsStorage {
	return &MockOrgSettingsStorage{
		settings: make(map[string]*model.OrgSettings),
		revision: make(map[string]uint64),
	}
}

// Seed pre-populates the mock with a settings value and revision for the given orgUID.
func (m *MockOrgSettingsStorage) Seed(orgUID string, s *model.OrgSettings, rev uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[orgUID] = s
	m.revision[orgUID] = rev
}

// SetPutError configures the mock to return err on the next PutOrgSettings call.
func (m *MockOrgSettingsStorage) SetPutError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putErr = err
}

// GetOrgSettings returns the seeded settings for orgUID, or (nil, 0, nil) when absent.
func (m *MockOrgSettingsStorage) GetOrgSettings(_ context.Context, orgUID string) (*model.OrgSettings, uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.settings[orgUID]
	if !ok {
		return nil, 0, nil
	}
	return s, m.revision[orgUID], nil
}

// PutOrgSettings stores settings for orgUID. Optimistic-lock semantics are only
// lightly enforced here (non-zero revision must match stored revision).
func (m *MockOrgSettingsStorage) PutOrgSettings(_ context.Context, orgUID string, s *model.OrgSettings, revision uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		err := m.putErr
		m.putErr = nil
		return err
	}
	if revision > 0 {
		if stored, ok := m.revision[orgUID]; !ok || stored != revision {
			return errors.NewConflict("stale revision")
		}
	}
	m.settings[orgUID] = s
	m.revision[orgUID] = revision + 1
	return nil
}
