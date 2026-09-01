// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/events"
)

// UserResolver concentrates all auth-service user identity and profile lookups behind a
// small interface. Callers receive a fully enriched value; timeout management, fallback
// chains, and error normalization are handled internally.
type UserResolver struct {
	reader domain.UserReader
}

// NewUserResolver returns a UserResolver backed by the given UserReader.
func NewUserResolver(reader domain.UserReader) *UserResolver {
	return &UserResolver{reader: reader}
}

// ResolveRequestingUser reads the JWT principal from ctx and resolves it to a full UserInfo
// for stamping created_by/updated_by on resource writes. Returns nil when no principal is
// present in the context.
func (r *UserResolver) ResolveRequestingUser(ctx context.Context) *models.UserInfo {
	principal, _ := ctx.Value(constants.PrincipalContextID).(string)
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return nil
	}
	email, _ := ctx.Value(constants.EmailContextID).(string)
	email = strings.TrimSpace(email)

	if r.reader == nil {
		return &models.UserInfo{Username: principal, Email: email}
	}

	lookupCtx, cancel := context.WithTimeout(ctx, userProfileResolveTimeout)
	defer cancel()

	user := &models.UserInfo{Username: principal}
	meta, err := r.reader.UserMetadataByPrincipal(lookupCtx, principal)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve user profile for audit stamp; stamping username/email only",
			"username", principal, constants.ErrKey, err)
	} else if meta != nil {
		if name := strings.TrimSpace(meta.Name); name != "" {
			user.Name = name
		} else if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
			user.Name = full
		}
		user.Avatar = meta.Picture
	}

	if resolvedEmail, emailErr := r.reader.PrimaryEmailByUsername(lookupCtx, principal); emailErr != nil {
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

// EnrichAuditUser best-effort enriches sparse audit user records on read paths (e.g. legacy
// KV entries that carry only a username). Returns the original pointer when the profile is
// already complete or when enrichment cannot be performed; otherwise returns a new cloned and
// populated UserInfo so the original is never mutated.
func (r *UserResolver) EnrichAuditUser(ctx context.Context, user *models.UserInfo) *models.UserInfo {
	if user == nil || strings.TrimSpace(user.Username) == "" || r.reader == nil {
		return user
	}
	if auditUserProfileComplete(user) {
		return user
	}
	lookupCtx, cancel := context.WithTimeout(ctx, userProfileResolveTimeout)
	defer cancel()
	meta, err := r.reader.UserMetadataByPrincipal(lookupCtx, user.Username)
	if err != nil || meta == nil {
		enriched := models.CloneUserInfo(user)
		if enriched.Email == "" {
			if resolvedEmail, emailErr := r.reader.PrimaryEmailByUsername(lookupCtx, user.Username); emailErr == nil && resolvedEmail != "" {
				enriched.Email = resolvedEmail
				return enriched
			}
		}
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
		if resolvedEmail, emailErr := r.reader.PrimaryEmailByUsername(lookupCtx, user.Username); emailErr == nil {
			enriched.Email = resolvedEmail
		}
	}
	return enriched
}

// UsernameByEmail looks up the LFID username for the given email address.
// Returns ("", nil) when the reader is nil (test/noop path).
func (r *UserResolver) UsernameByEmail(ctx context.Context, email string) (string, error) {
	if r.reader == nil {
		return "", nil
	}
	return r.reader.UsernameByEmail(ctx, email)
}

// ResolveDisplayName returns the display name for the given actor, suitable for use in
// notification emails. It falls back to "A project member" when the actor carries no name
// and the auth-service lookup fails or returns empty.
func (r *UserResolver) ResolveDisplayName(ctx context.Context, actor events.Actor) string {
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Username != "" && r.reader != nil {
		lookupCtx, cancel := context.WithTimeout(ctx, notificationTimeout)
		defer cancel()
		if meta, err := r.reader.UserMetadataByPrincipal(lookupCtx, actor.Username); err == nil && meta != nil {
			if meta.Name != "" {
				return meta.Name
			}
			if full := strings.TrimSpace(meta.GivenName + " " + meta.FamilyName); full != "" {
				return full
			}
		}
	}
	return "A project member"
}

// auditUserProfileComplete reports whether a UserInfo record is already fully enriched
// (name, avatar, and email are all non-empty).
func auditUserProfileComplete(u *models.UserInfo) bool {
	return strings.TrimSpace(u.Name) != "" &&
		strings.TrimSpace(u.Avatar) != "" &&
		strings.TrimSpace(u.Email) != ""
}
