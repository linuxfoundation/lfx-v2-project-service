// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

const userProfileResolveTimeout = 2 * time.Second

// normalizeAuditUsers ensures legacy flat fields are migrated in-memory and enriches when needed.
func (s *ProjectsService) normalizeAuditUsers(ctx context.Context, createdBy, updatedBy *models.UserInfo, legacyCreatedByUsername, legacyUploadedByUsername string) (*models.UserInfo, *models.UserInfo) {
	createdBy, updatedBy = models.NormalizeLegacyAuditUsers(createdBy, updatedBy, legacyCreatedByUsername, legacyUploadedByUsername)
	if createdBy != nil {
		createdBy = s.Resolver.EnrichAuditUser(ctx, createdBy)
	}
	if updatedBy == nil && createdBy != nil {
		updatedBy = models.CloneUserInfo(createdBy)
	} else if updatedBy != nil {
		updatedBy = s.Resolver.EnrichAuditUser(ctx, updatedBy)
	}
	return createdBy, updatedBy
}

// stampAuditUsers sets created_by and updated_by from the requesting user on resource writes.
func (s *ProjectsService) stampAuditUsers(ctx context.Context) (*models.UserInfo, *models.UserInfo) {
	creator := s.Resolver.ResolveRequestingUser(ctx)
	if creator == nil {
		return nil, nil
	}
	updated := models.CloneUserInfo(creator)
	return creator, updated
}

// ResolveAuditUserProfile best-effort resolves a username into a full UserInfo via auth-service.
func ResolveAuditUserProfile(ctx context.Context, reader domain.UserReader, username string) *models.UserInfo {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	return NewUserResolver(reader).EnrichAuditUser(ctx, &models.UserInfo{Username: username})
}
