// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

func (s *NatsRepository) links() kvCollection[models.ProjectLink] {
	return kvCollection[models.ProjectLink]{
		kv:         s.Links,
		noun:       "link",
		notFound:   domain.ErrLinkNotFound,
		projectUID: func(l *models.ProjectLink) string { return l.ProjectUID },
	}
}

// GetLink gets a project link, verifying it belongs to the given project.
func (s *NatsRepository) GetLink(ctx context.Context, projectUID, linkUID string) (*models.ProjectLink, uint64, error) {
	return s.links().getForProject(ctx, projectUID, linkUID)
}

// ListLinks lists all links for a given project by scanning all bucket keys and
// filtering client-side on the per-project index prefix. NATS KV ListKeys does not
// support server-side prefix filtering, so this is still O(total keys in the bucket).
func (s *NatsRepository) ListLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	prefix := fmt.Sprintf("lookup/project-links/%s/", projectUID)
	keysLister, err := s.Links.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing link keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	links := []*models.ProjectLink{}
	for key := range keysLister.Keys() {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		linkUID := strings.TrimPrefix(key, prefix)
		link, _, err := s.links().get(ctx, linkUID)
		if err != nil {
			if errors.Is(err, domain.ErrLinkNotFound) {
				// Stale index key — backing record was deleted; skip and continue.
				slog.WarnContext(ctx, "stale link index key found, skipping", "link_uid", linkUID)
				continue
			}
			return nil, err
		}

		links = append(links, link)
	}

	return links, nil
}

// CreateLink stores a new project link and writes a per-project index key.
func (s *NatsRepository) CreateLink(ctx context.Context, link *models.ProjectLink) error {
	if err := s.links().put(ctx, link.UID, link); err != nil {
		return err
	}

	// Write the per-project index key so ListLinks can filter without a full scan.
	// If this fails, roll back the link record so the store stays consistent.
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID)
	if _, err := s.Links.Put(ctx, lookupKey, []byte(link.UID)); err != nil {
		slog.ErrorContext(ctx, "error writing link index key to NATS KV store; rolling back link record", constants.ErrKey, err, "key", lookupKey)
		if rbErr := s.Links.Purge(ctx, link.UID); rbErr != nil {
			slog.ErrorContext(ctx, "error rolling back link record from NATS KV store", constants.ErrKey, rbErr, "link_uid", link.UID)
		}
		return domain.ErrInternal
	}

	return nil
}

// DeleteLink deletes a project link with optimistic concurrency and removes its index key.
func (s *NatsRepository) DeleteLink(ctx context.Context, projectUID, linkUID string, revision uint64) error {
	link, _, err := s.GetLink(ctx, projectUID, linkUID)
	if err != nil {
		return err
	}

	if err = s.links().deleteAt(ctx, linkUID, revision); err != nil {
		return err
	}

	// Clean up the per-project index key (fire-and-forget; log on failure)
	lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID)
	if err = s.Links.Purge(ctx, lookupKey); err != nil {
		slog.WarnContext(ctx, "error purging link index key from NATS KV store", constants.ErrKey, err, "key", lookupKey)
	}

	return nil
}

// ListAllLinks lists project links. When projectUID is non-empty, only that project's links are returned.
func (s *NatsRepository) ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error) {
	if projectUID != "" {
		return s.ListLinks(ctx, projectUID)
	}
	return s.links().list(ctx, func(*models.ProjectLink) bool { return true })
}

// UpdateLink persists link metadata with optimistic concurrency.
func (s *NatsRepository) UpdateLink(ctx context.Context, link *models.ProjectLink, revision uint64) error {
	return s.links().update(ctx, link.UID, link, revision)
}
