// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const userProfileResolveTimeout = 2 * time.Second

// resolveRequestingUser resolves the requesting principal from ctx into a UserInfo suitable
// for stamping created_by / updated_by on document resources. Mirrors meeting-service auditStamper.
func (s *ProjectsService) resolveRequestingUser(ctx context.Context) *models.UserInfo {
	principal, _ := ctx.Value(constants.PrincipalContextID).(string)
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return nil
	}
	email, _ := ctx.Value(constants.EmailContextID).(string)
	email = strings.TrimSpace(email)

	if s.UserReader == nil {
		return &models.UserInfo{Username: principal, Email: email}
	}

	lookupCtx, cancel := context.WithTimeout(ctx, userProfileResolveTimeout)
	defer cancel()

	meta, err := s.UserReader.UserMetadataByPrincipal(lookupCtx, principal)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve user profile for audit stamp; stamping username/email only",
			"username", principal, constants.ErrKey, err)
		return &models.UserInfo{Username: principal, Email: email}
	}

	user := &models.UserInfo{Username: principal}
	if meta != nil {
		user.Name = meta.Name
		if user.Name == "" {
			user.Name = strings.TrimSpace(meta.GivenName + " " + meta.FamilyName)
		}
		user.Avatar = meta.Picture
	}

	if resolvedEmail, emailErr := s.UserReader.PrimaryEmailByUsername(lookupCtx, principal); emailErr != nil {
		slog.WarnContext(ctx, "failed to resolve email for audit stamp; using JWT email if present",
			"username", principal, constants.ErrKey, emailErr)
	} else if resolvedEmail != "" {
		user.Email = resolvedEmail
	}
	if user.Email == "" {
		user.Email = email
	}

	return user
}

// enrichAuditUserIfMissing best-effort enriches an audit user when name is absent (legacy records).
func (s *ProjectsService) enrichAuditUserIfMissing(ctx context.Context, user *models.UserInfo) *models.UserInfo {
	if user == nil || strings.TrimSpace(user.Username) == "" || strings.TrimSpace(user.Name) != "" || s.UserReader == nil {
		return user
	}
	lookupCtx, cancel := context.WithTimeout(ctx, userProfileResolveTimeout)
	defer cancel()
	meta, err := s.UserReader.UserMetadataByPrincipal(lookupCtx, user.Username)
	if err != nil || meta == nil {
		return user
	}
	enriched := models.CloneUserInfo(user)
	if meta.Name != "" {
		enriched.Name = meta.Name
	} else if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
		enriched.Name = full
	}
	if enriched.Avatar == "" {
		enriched.Avatar = meta.Picture
	}
	if enriched.Email == "" {
		if resolvedEmail, emailErr := s.UserReader.PrimaryEmailByUsername(lookupCtx, user.Username); emailErr == nil {
			enriched.Email = resolvedEmail
		}
	}
	return enriched
}

// normalizeDocumentAuditUsers ensures legacy flat fields are migrated in-memory and enriches when needed.
func (s *ProjectsService) normalizeDocumentAuditUsers(ctx context.Context, createdBy, updatedBy **models.UserInfo, legacyCreatedByUsername, legacyUploadedByUsername string) {
	models.NormalizeLegacyAuditUsers(createdBy, updatedBy, legacyCreatedByUsername, legacyUploadedByUsername)
	if createdBy != nil && *createdBy != nil {
		*createdBy = s.enrichAuditUserIfMissing(ctx, *createdBy)
	}
	if updatedBy != nil && createdBy != nil && *createdBy != nil {
		if *updatedBy == nil || (*updatedBy).Username == (*createdBy).Username {
			// Legacy records and create-time stamps keep both fields identical.
			*updatedBy = models.CloneUserInfo(*createdBy)
		} else {
			*updatedBy = s.enrichAuditUserIfMissing(ctx, *updatedBy)
		}
	}
}

// stampDocumentAuditUsers sets created_by and updated_by on a new resource from the requesting user.
func (s *ProjectsService) stampDocumentAuditUsers(ctx context.Context) (*models.UserInfo, *models.UserInfo) {
	creator := s.resolveRequestingUser(ctx)
	if creator == nil {
		return nil, nil
	}
	updated := models.CloneUserInfo(creator)
	return creator, updated
}
