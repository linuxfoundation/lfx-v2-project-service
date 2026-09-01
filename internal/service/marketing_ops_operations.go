// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"log/slog"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"golang.org/x/sync/errgroup"
)

// AddMarketingOpsMember grants a user Marketing Ops access (Campaign Impact and
// Campaigns) scoped to a single project. It publishes two messages:
//  1. update_access rebuilt from the project's current DB state, which always
//     includes the per-project marketing_ops team reference (see
//     buildFGAUpdateAccessMessage) — this backfills the team->project tuple for
//     projects created before this feature and re-asserts it idempotently.
//  2. member_put adding the user to that project's marketing ops team.
func (s *ProjectsService) AddMarketingOpsMember(ctx context.Context, projectUID, username string) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return domain.ErrServiceUnavailable
	}

	if username == "" {
		return domain.ErrValidationFailed
	}

	if _, err := s.UserReader.UserMetadataByPrincipal(ctx, username); err != nil {
		slog.WarnContext(ctx, "marketing ops grant rejected: username does not resolve to a known user",
			constants.ErrKey, err)
		return domain.ErrValidationFailed
	}

	projectDB, err := s.ProjectRepository.GetProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			return domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project base", constants.ErrKey, err)
		return domain.ErrInternal
	}

	projectSettingsDB, err := s.ProjectRepository.GetProjectSettings(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error getting project settings", constants.ErrKey, err)
		return domain.ErrInternal
	}

	proj := NewProjectProjection(projectDB, projectSettingsDB)

	g := new(errgroup.Group)
	g.Go(func() error {
		return s.MessageBuilder.PublishAccessMessage(ctx, fgaconstants.GenericUpdateAccessSubject, proj.ToFGAMessage())
	})
	g.Go(func() error {
		return s.MessageBuilder.PublishAccessMessage(ctx, fgaconstants.GenericMemberPutSubject, fgatypes.GenericFGAMessage{
			ObjectType: "team",
			Operation:  "member_put",
			Data: fgatypes.GenericMemberData{
				UID:       marketingOpsTeamID(projectUID),
				Username:  username,
				Relations: []string{fgaconstants.RelationMember},
			},
		})
	})

	if err := g.Wait(); err != nil {
		slog.ErrorContext(ctx, "error publishing marketing ops membership grant", constants.ErrKey, err)
		return domain.ErrInternal
	}

	return nil
}

// RemoveMarketingOpsMember revokes a user's Marketing Ops access for a single
// project by removing them from that project's marketing ops team. The
// team->project grant tuple (update_access reference) intentionally remains —
// access is controlled purely by team membership.
func (s *ProjectsService) RemoveMarketingOpsMember(ctx context.Context, projectUID, username string) error {
	if !s.ServiceReady() {
		slog.ErrorContext(ctx, "service not ready")
		return domain.ErrServiceUnavailable
	}

	if username == "" {
		return domain.ErrValidationFailed
	}

	exists, err := s.ProjectRepository.ProjectExists(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error checking if project exists", constants.ErrKey, err)
		return domain.ErrInternal
	}
	if !exists {
		return domain.ErrProjectNotFound
	}

	err = s.MessageBuilder.PublishAccessMessage(ctx, fgaconstants.GenericMemberRemoveSubject, fgatypes.GenericFGAMessage{
		ObjectType: "team",
		Operation:  "member_remove",
		Data: fgatypes.GenericMemberData{
			UID:       marketingOpsTeamID(projectUID),
			Username:  username,
			Relations: []string{fgaconstants.RelationMember},
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "error publishing marketing ops membership revoke", constants.ErrKey, err)
		return domain.ErrInternal
	}

	return nil
}
