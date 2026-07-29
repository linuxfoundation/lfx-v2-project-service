// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const userProfileResolveTimeout = 2 * time.Second

// resolveRequestingUser resolves the JWT principal into a UserInfo for stamping created_by/updated_by.
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
		if name := strings.TrimSpace(meta.Name); name != "" {
			user.Name = name
		} else if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
			user.Name = full
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

// enrichAuditUserIfMissing best-effort enriches sparse audit users on read paths (legacy KV records).
func (s *ProjectsService) enrichAuditUserIfMissing(ctx context.Context, user *models.UserInfo) *models.UserInfo {
	if user == nil || strings.TrimSpace(user.Username) == "" || s.UserReader == nil {
		return user
	}
	if auditUserProfileComplete(user) {
		return user
	}
	lookupCtx, cancel := context.WithTimeout(ctx, userProfileResolveTimeout)
	defer cancel()
	meta, err := s.UserReader.UserMetadataByPrincipal(lookupCtx, user.Username)
	if err != nil || meta == nil {
		return user
	}
	enriched := models.CloneUserInfo(user)
	if strings.TrimSpace(enriched.Name) == "" {
		if name := strings.TrimSpace(meta.Name); name != "" {
			enriched.Name = name
		} else if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
			enriched.Name = full
		}
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

func auditUserProfileComplete(u *models.UserInfo) bool {
	return strings.TrimSpace(u.Name) != "" &&
		strings.TrimSpace(u.Avatar) != "" &&
		strings.TrimSpace(u.Email) != ""
}

// normalizeAuditUsers ensures legacy flat fields are migrated in-memory and enriches when needed.
func (s *ProjectsService) normalizeAuditUsers(ctx context.Context, createdBy, updatedBy *models.UserInfo, legacyCreatedByUsername, legacyUploadedByUsername string) (*models.UserInfo, *models.UserInfo) {
	createdBy, updatedBy = models.NormalizeLegacyAuditUsers(createdBy, updatedBy, legacyCreatedByUsername, legacyUploadedByUsername)
	if createdBy != nil {
		createdBy = s.enrichAuditUserIfMissing(ctx, createdBy)
	}
	if updatedBy == nil && createdBy != nil {
		updatedBy = models.CloneUserInfo(createdBy)
	} else if updatedBy != nil {
		updatedBy = s.enrichAuditUserIfMissing(ctx, updatedBy)
	}
	return createdBy, updatedBy
}

// stampAuditUsers sets created_by and updated_by from the requesting user on resource writes.
func (s *ProjectsService) stampAuditUsers(ctx context.Context) (*models.UserInfo, *models.UserInfo) {
	creator := s.resolveRequestingUser(ctx)
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
	svc := &ProjectsService{UserReader: reader}
	return svc.enrichAuditUserIfMissing(ctx, &models.UserInfo{Username: username})
}
