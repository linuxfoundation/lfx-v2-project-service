// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

func TestParseDocumentResourceType(t *testing.T) {
	t.Run("empty means all", func(t *testing.T) {
		folders, links, docs, err := parseDocumentResourceType("")
		require.NoError(t, err)
		assert.True(t, folders)
		assert.True(t, links)
		assert.True(t, docs)
	})

	t.Run("folder only", func(t *testing.T) {
		folders, links, docs, err := parseDocumentResourceType("folder")
		require.NoError(t, err)
		assert.True(t, folders)
		assert.False(t, links)
		assert.False(t, docs)
	})

	t.Run("invalid", func(t *testing.T) {
		_, _, _, err := parseDocumentResourceType("bad")
		assert.Error(t, err)
	})
}

func TestDocumentAuditUsersRunner_applyAuditUsers_skipsEnriched(t *testing.T) {
	runner := &documentAuditUsersRunner{
		stats: commands.NewStats(),
	}
	err := runner.applyAuditUsers(context.Background(), freshAuditResource{
		resourceType: "folder",
		uid:          "f1",
		projectUID:   "p1",
		createdBy:    &models.UserInfo{Username: "alice", Name: "Alice Example"},
	}, func(*models.UserInfo) error {
		t.Fatal("apply should not be called")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, runner.stats.Skipped)
}

func TestDocumentAuditUsersRunner_applyAuditUsers_dryRun(t *testing.T) {
	mockUser := &domain.MockUserReader{}
	mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
		Name: "Alice Example",
	}, nil)
	mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("", nil)

	runner := &documentAuditUsersRunner{
		userReader: mockUser,
		dryRun:     true,
		stats:      commands.NewStats(),
	}
	err := runner.applyAuditUsers(context.Background(), freshAuditResource{
		resourceType: "link",
		uid:          "l1",
		projectUID:   "p1",
		createdBy:    &models.UserInfo{Username: "alice"},
	}, func(*models.UserInfo) error {
		t.Fatal("apply should not be called in dry-run")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, runner.stats.Updated)
	mockUser.AssertExpectations(t)
}

func TestDocumentAuditUsersRunner_applyAuditUsers_writes(t *testing.T) {
	mockUser := &domain.MockUserReader{}
	mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
		Name: "Alice Example",
	}, nil)
	mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("", nil)

	var applied bool
	runner := &documentAuditUsersRunner{
		userReader: mockUser,
		dryRun:     false,
		stats:      commands.NewStats(),
	}
	err := runner.applyAuditUsers(context.Background(), freshAuditResource{
		resourceType: "document",
		uid:          "d1",
		projectUID:   "p1",
		createdBy:    &models.UserInfo{Username: "alice"},
	}, func(profile *models.UserInfo) error {
		applied = true
		assert.Equal(t, "Alice Example", profile.Name)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, 1, runner.stats.Updated)
	mockUser.AssertExpectations(t)
}
