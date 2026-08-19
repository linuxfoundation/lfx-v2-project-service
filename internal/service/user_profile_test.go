// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

func TestUserResolver_ResolveRequestingUser(t *testing.T) {
	t.Run("nil principal returns nil", func(t *testing.T) {
		resolver := NewUserResolver(&domain.MockUserReader{})
		got := resolver.ResolveRequestingUser(context.Background())
		assert.Nil(t, got)
	})

	t.Run("no user reader stamps username and email from context", func(t *testing.T) {
		resolver := NewUserResolver(nil)
		ctx := context.WithValue(context.Background(), constants.PrincipalContextID, "alice")
		ctx = context.WithValue(ctx, constants.EmailContextID, "alice@example.com")

		got := resolver.ResolveRequestingUser(ctx)

		assert.NotNil(t, got)
		assert.Equal(t, "alice", got.Username)
		assert.Equal(t, "alice@example.com", got.Email)
	})

	t.Run("happy path resolves full profile", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
			Name:    "Alice Example",
			Picture: "https://cdn.example/avatar.png",
		}, nil)
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("alice@lf.org", nil)

		resolver := NewUserResolver(mockUser)
		ctx := context.WithValue(context.Background(), constants.PrincipalContextID, "alice")
		ctx = context.WithValue(ctx, constants.EmailContextID, "jwt@example.com")

		got := resolver.ResolveRequestingUser(ctx)

		assert.NotNil(t, got)
		assert.Equal(t, "alice", got.Username)
		assert.Equal(t, "Alice Example", got.Name)
		assert.Equal(t, "https://cdn.example/avatar.png", got.Avatar)
		assert.Equal(t, "alice@lf.org", got.Email)
		mockUser.AssertExpectations(t)
	})

	t.Run("metadata failure falls back to username and JWT email", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(nil, errors.New("nats unavailable"))
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("", nil)

		resolver := NewUserResolver(mockUser)
		ctx := context.WithValue(context.Background(), constants.PrincipalContextID, "alice")
		ctx = context.WithValue(ctx, constants.EmailContextID, "jwt@example.com")

		got := resolver.ResolveRequestingUser(ctx)

		assert.NotNil(t, got)
		assert.Equal(t, "alice", got.Username)
		assert.Equal(t, "jwt@example.com", got.Email)
		assert.Empty(t, got.Name)
		mockUser.AssertExpectations(t)
	})

	t.Run("given and family name used when name empty", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "bob").Return(&domain.UserMetadata{
			GivenName:  "Bob",
			FamilyName: "Smith",
		}, nil)
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "bob").Return("", nil)

		resolver := NewUserResolver(mockUser)
		ctx := context.WithValue(context.Background(), constants.PrincipalContextID, "bob")

		got := resolver.ResolveRequestingUser(ctx)

		assert.Equal(t, "Bob Smith", got.Name)
		mockUser.AssertExpectations(t)
	})
}

func TestUserResolver_EnrichAuditUser(t *testing.T) {
	t.Run("skips when profile is complete", func(t *testing.T) {
		resolver := NewUserResolver(&domain.MockUserReader{})
		user := &models.UserInfo{Username: "alice", Name: "Alice Example", Avatar: "a.png", Email: "a@lf.org"}

		got := resolver.EnrichAuditUser(context.Background(), user)

		assert.Equal(t, user, got)
	})

	t.Run("enriches legacy username-only record", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
			Name:    "Alice Example",
			Picture: "https://cdn.example/avatar.png",
		}, nil)
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("alice@lf.org", nil)

		resolver := NewUserResolver(mockUser)
		user := &models.UserInfo{Username: "alice"}
		got := resolver.EnrichAuditUser(context.Background(), user)

		assert.Equal(t, "Alice Example", got.Name)
		assert.Equal(t, "https://cdn.example/avatar.png", got.Avatar)
		assert.Equal(t, "alice@lf.org", got.Email)
		assert.Equal(t, &models.UserInfo{Username: "alice"}, user)
		assert.NotSame(t, user, got)
		mockUser.AssertExpectations(t)
	})

	t.Run("enriches email when metadata lookup fails", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(nil, errors.New("timeout"))
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("alice@lf.org", nil)

		resolver := NewUserResolver(mockUser)
		user := &models.UserInfo{Username: "alice"}
		got := resolver.EnrichAuditUser(context.Background(), user)

		assert.Equal(t, "alice@lf.org", got.Email)
		assert.NotSame(t, user, got)
		mockUser.AssertExpectations(t)
	})

	t.Run("returns unchanged user on lookup failure", func(t *testing.T) {
		mockUser := &domain.MockUserReader{}
		mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(nil, errors.New("timeout"))
		mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("", nil)

		resolver := NewUserResolver(mockUser)
		user := &models.UserInfo{Username: "alice"}
		got := resolver.EnrichAuditUser(context.Background(), user)

		assert.Equal(t, user, got)
		mockUser.AssertExpectations(t)
	})
}

func TestProjectsService_stampAuditUsers(t *testing.T) {
	svc, _, _, _ := setupServiceForTesting()
	mockUser := svc.Resolver.reader.(*domain.MockUserReader)
	mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
		Name: "Alice Example",
	}, nil)
	mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("alice@lf.org", nil)

	ctx := context.WithValue(context.Background(), constants.PrincipalContextID, "alice")
	created, updated := svc.stampAuditUsers(ctx)

	assert.NotNil(t, created)
	assert.NotNil(t, updated)
	assert.Equal(t, created.Username, updated.Username)
	assert.Equal(t, created.Name, updated.Name)
	assert.Equal(t, "Alice Example", created.Name)
	mockUser.AssertExpectations(t)
}

func TestProjectsService_normalizeAuditUsers(t *testing.T) {
	svc, _, _, _ := setupServiceForTesting()
	mockUser := svc.Resolver.reader.(*domain.MockUserReader)
	mockUser.On("UserMetadataByPrincipal", mock.Anything, "alice").Return(&domain.UserMetadata{
		Name: "Alice Example",
	}, nil)
	mockUser.On("PrimaryEmailByUsername", mock.Anything, "alice").Return("", nil)

	createdBy, updatedBy := svc.normalizeAuditUsers(context.Background(), nil, nil, "alice", "")

	assert.NotNil(t, createdBy)
	assert.Equal(t, "alice", createdBy.Username)
	assert.Equal(t, "Alice Example", createdBy.Name)
	assert.NotNil(t, updatedBy)
	assert.Equal(t, createdBy.Name, updatedBy.Name)
	mockUser.AssertExpectations(t)
}
