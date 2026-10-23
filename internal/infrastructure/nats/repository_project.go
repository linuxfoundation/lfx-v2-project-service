// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

func (s *NatsRepository) getProjectBase(ctx context.Context, projectUID string) (jetstream.KeyValueEntry, error) {
	entry, err := s.Projects.Get(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *NatsRepository) getProjectBaseUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectBase, error) {
	projectDB := models.ProjectBase{}
	err := json.Unmarshal(entry.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return &projectDB, nil
}

// GetProjectBase gets the project base from the NATS KV store.
func (s *NatsRepository) GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error) {
	entry, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", projectUID)
		return nil, domain.ErrInternal
	}

	projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
	if err != nil {
		return nil, domain.ErrUnmarshal
	}

	return projectDB, nil
}

// GetProjectBaseWithRevision gets the project base from the NATS KV store along with its revision.
func (s *NatsRepository) GetProjectBaseWithRevision(ctx context.Context, projectUID string) (*models.ProjectBase, uint64, error) {
	entry, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, 0, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", projectUID)
		return nil, 0, domain.ErrInternal
	}

	projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, domain.ErrUnmarshal
	}

	return projectDB, entry.Revision(), nil
}

// ProjectExists checks if a project exists in the NATS KV store.
func (s *NatsRepository) ProjectExists(ctx context.Context, projectUID string) (bool, error) {
	_, err := s.getProjectBase(ctx, projectUID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, domain.ErrInternal
	}

	return true, nil
}

// GetProjectUIDFromSlug gets the project UID from the project slug.
func (s *NatsRepository) GetProjectUIDFromSlug(ctx context.Context, projectSlug string) (projectUID string, err error) {
	var entry jetstream.KeyValueEntry
	entry, err = s.Projects.Get(ctx, fmt.Sprintf(constants.KVSlugKey, projectSlug))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err)
		return "", domain.ErrInternal
	}

	return string(entry.Value()), nil
}

// ProjectSlugExists checks if a project slug exists in the NATS KV store.
func (s *NatsRepository) ProjectSlugExists(ctx context.Context, projectSlug string) (bool, error) {
	_, err := s.GetProjectUIDFromSlug(ctx, projectSlug)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			// We only care about knowing the existence of the slug, so this is not an error
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ListAllProjectsBase lists all project base data from the NATS KV stores.
func (s *NatsRepository) ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error) {
	keysLister, err := s.Projects.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectsBase := []*models.ProjectBase{}
	for key := range keysLister.Keys() {
		// Skip slug mappings
		if strings.HasPrefix(key, constants.KVSlugPrefix) {
			continue
		}

		entry, err := s.getProjectBase(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrInternal
		}

		projectDB, err := s.getProjectBaseUnmarshal(ctx, entry)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrUnmarshal
		}

		projectsBase = append(projectsBase, projectDB)
	}

	return projectsBase, nil
}

// ListAllProjectsSettings lists all project settings data from the NATS KV stores.
func (s *NatsRepository) ListAllProjectsSettings(ctx context.Context) ([]*models.ProjectSettings, error) {
	keysLister, err := s.ProjectSettings.ListKeys(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error listing project settings keys from NATS KV store", constants.ErrKey, err)
		return nil, domain.ErrInternal
	}

	projectsSettings := []*models.ProjectSettings{}
	for key := range keysLister.Keys() {
		if strings.HasPrefix(key, constants.KVLookupPrefix) {
			continue
		}
		entry, err := s.ProjectSettings.Get(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "error getting project settings from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrInternal
		}

		projectSettingsDB := &models.ProjectSettings{}
		err = json.Unmarshal(entry.Value(), projectSettingsDB)
		if err != nil {
			slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", constants.ErrKey, err, "project_uid", key)
			return nil, domain.ErrUnmarshal
		}

		projectsSettings = append(projectsSettings, projectSettingsDB)
	}

	return projectsSettings, nil
}

// ListAllProjects lists all projects from the NATS KV stores.
func (s *NatsRepository) ListAllProjects(ctx context.Context) ([]*models.ProjectBase, []*models.ProjectSettings, error) {
	projectsBase, err := s.ListAllProjectsBase(ctx)
	if err != nil {
		return nil, nil, err
	}

	projectsSettings, err := s.ListAllProjectsSettings(ctx)
	if err != nil {
		return nil, nil, err
	}

	return projectsBase, projectsSettings, nil
}

func (s *NatsRepository) putProjectBase(ctx context.Context, projectBase *models.ProjectBase) (uint64, error) {
	projectBaseBytes, err := json.Marshal(projectBase)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", constants.ErrKey, err)
		return 0, err
	}

	revision, err := s.Projects.Put(ctx, projectBase.UID, projectBaseBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project into NATS KV store", constants.ErrKey, err)
		return 0, err
	}

	return revision, nil
}

func (s *NatsRepository) putProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings) (uint64, error) {
	projectSettingsBytes, err := json.Marshal(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project settings into JSON", constants.ErrKey, err)
		return 0, err
	}

	revision, err := s.ProjectSettings.Put(ctx, projectSettings.UID, projectSettingsBytes)
	if err != nil {
		slog.ErrorContext(ctx, "error putting project settings into NATS KV store", constants.ErrKey, err)
		return 0, err
	}

	return revision, nil
}

// CreateProject creates a new project in the NATS KV stores.
func (s *NatsRepository) CreateProject(ctx context.Context, projectBase *models.ProjectBase, projectSettings *models.ProjectSettings) error {
	// Reserve the slug atomically. Create fails with ErrKeyExists when the key is
	// already present, so concurrent creates for the same slug yield exactly one
	// winner rather than silently overwriting each other.
	slugKey := fmt.Sprintf(constants.KVSlugKey, projectBase.Slug)
	_, err := s.Projects.Create(ctx, slugKey, []byte(projectBase.UID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			slog.WarnContext(ctx, "project slug already exists", constants.ErrKey, err)
			return domain.ErrProjectSlugExists
		}
		slog.ErrorContext(ctx, "error creating project slug mapping in NATS KV store", constants.ErrKey, err)
		return domain.ErrInternal
	}

	// Store the project base data. A Put can fail with a lost acknowledgement:
	// the write may already be committed server-side. Re-read before releasing
	// the slug reservation so we only roll back when non-commit is confirmed.
	_, err = s.putProjectBase(ctx, projectBase)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, gErr := s.GetProjectBase(cleanupCtx, projectBase.UID); errors.Is(gErr, domain.ErrProjectNotFound) {
			// Base confirmed not committed — safe to release the slug reservation.
			if delErr := s.deleteProjectSlugMapping(cleanupCtx, projectBase.Slug, projectBase.UID); delErr != nil {
				slog.ErrorContext(cleanupCtx, "error rolling back slug mapping after base write failure",
					constants.ErrKey, delErr, "slug", projectBase.Slug, "project_uid", projectBase.UID)
			}
		} else {
			// Base may be committed or the read itself failed — keep the slug
			// reservation so the project remains reachable; log for investigation.
			slog.ErrorContext(cleanupCtx, "base write failed; slug reservation kept pending retry or manual cleanup",
				constants.ErrKey, err, "slug", projectBase.Slug, "project_uid", projectBase.UID)
		}
		return domain.ErrInternal
	}

	// Store the project settings if provided. The base is now confirmed committed;
	// do not roll back the base or slug on settings failure — they belong to a
	// real project. Return ErrInternal so the caller can retry the settings write.
	if projectSettings != nil {
		_, err = s.putProjectSettings(ctx, projectSettings)
		if err != nil {
			slog.ErrorContext(ctx, "error writing project settings; base committed, retrying settings is safe",
				constants.ErrKey, err, "project_uid", projectBase.UID)
			return domain.ErrInternal
		}
	}

	return nil
}

func (s *NatsRepository) updateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error {
	projectBaseBytes, err := json.Marshal(projectBase)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project into JSON", constants.ErrKey, err)
		return err
	}

	_, err = s.Projects.Update(ctx, projectBase.UID, projectBaseBytes, revision)
	if err != nil {
		slog.ErrorContext(ctx, "error updating project in NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

// UpdateProjectBase updates a project's base information in the NATS KV store.
func (s *NatsRepository) UpdateProjectBase(ctx context.Context, projectBase *models.ProjectBase, revision uint64) error {
	// Get the existing project to check if slug is changing
	existingProject, err := s.GetProjectBase(ctx, projectBase.UID)
	if err != nil {
		return err
	}

	slugChanging := existingProject.Slug != projectBase.Slug
	// weCreatedReservation tracks whether THIS call is the confirmed creator of the
	// slug/<new> KV entry. It must be false when we enter via the idempotent-retry
	// path (ErrKeyExists + matching UID) because a concurrent request may have
	// created the entry and already won the CAS; rolling back a reservation we did
	// not create would delete that winning request's valid index entry.
	weCreatedReservation := false
	if slugChanging {
		// Reserve the new slug BEFORE the CAS update so a stale-If-Match failure can be
		// rolled back cleanly. Use Create (conditional) to avoid overwriting another
		// project's mapping if the index is already taken.
		newSlugKey := fmt.Sprintf(constants.KVSlugKey, projectBase.Slug)
		_, createErr := s.Projects.Create(ctx, newSlugKey, []byte(projectBase.UID))
		if createErr != nil {
			if errors.Is(createErr, jetstream.ErrKeyExists) {
				// Idempotent-retry path: an entry already exists. Read it to decide
				// whether it belongs to us (prior crashed attempt) or another project.
				// Treat a Get failure as an infra error rather than a slug conflict.
				e, gerr := s.Projects.Get(ctx, newSlugKey)
				if gerr != nil {
					slog.ErrorContext(ctx, "error verifying slug reservation", constants.ErrKey, gerr)
					return domain.ErrInternal
				}
				if string(e.Value()) != projectBase.UID {
					return domain.ErrProjectSlugExists
				}
				// Pre-existing reservation for our UID — proceed but leave
				// weCreatedReservation false; we must not roll it back on CAS failure
				// because a concurrent request may be the actual owner.
			} else {
				slog.ErrorContext(ctx, "error creating new slug mapping", constants.ErrKey, createErr)
				return domain.ErrInternal
			}
		} else {
			weCreatedReservation = true
		}
	}

	// Update the project base data with CAS revision check.
	err = s.updateProjectBase(ctx, projectBase, revision)
	if err != nil {
		revMismatch := isJetStreamCASConflict(err)
		// Only roll back the new slug reservation when we are its confirmed creator.
		// Rolling back an inherited reservation risks destroying a concurrent winner's mapping.
		if slugChanging && weCreatedReservation {
			// Preserve request-scoped log values; strip only the cancellation signal so
			// cleanup can complete even after the caller's context is cancelled.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()

			if revMismatch {
				// Revision mismatch guarantees the write did not go through — safe to roll back.
				if delErr := s.deleteProjectSlugMapping(cleanupCtx, projectBase.Slug, projectBase.UID); delErr != nil {
					slog.ErrorContext(cleanupCtx, "error rolling back new slug mapping after CAS failure", constants.ErrKey, delErr)
				}
			} else {
				// For other errors (network timeout, etc.) the server may have already applied
				// the write. Re-read to confirm the slug was NOT committed before rolling back.
				if current, gErr := s.GetProjectBase(cleanupCtx, projectBase.UID); gErr == nil && current.Slug != projectBase.Slug {
					if delErr := s.deleteProjectSlugMapping(cleanupCtx, projectBase.Slug, projectBase.UID); delErr != nil {
						slog.ErrorContext(cleanupCtx, "error rolling back new slug mapping after non-CAS failure", constants.ErrKey, delErr)
					}
				} else if gErr != nil {
					slog.ErrorContext(cleanupCtx, "error re-reading project to determine slug rollback; reservation kept",
						constants.ErrKey, gErr, "slug", projectBase.Slug, "project_uid", projectBase.UID)
				}
			}
		}
		if revMismatch {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	// CAS succeeded — remove the old slug mapping. Use a context that preserves
	// request-scoped log values but is not cancelled when the caller's context ends.
	if slugChanging {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err = s.deleteProjectSlugMapping(cleanupCtx, existingProject.Slug, projectBase.UID); err != nil {
			slog.ErrorContext(cleanupCtx, "error deleting old slug mapping after successful update",
				constants.ErrKey, err,
				"event", "orphaned_slug_index",
				"old_slug", existingProject.Slug,
				"project_uid", projectBase.UID)
		}
	}

	return nil
}

func (s *NatsRepository) getProjectSettings(ctx context.Context, projectUID string) (jetstream.KeyValueEntry, error) {
	entry, err := s.ProjectSettings.Get(ctx, projectUID)
	if err != nil {
		// A missing record is translated to the domain sentinel here rather than in
		// each entry point, as GetProjectBase does for projects, so a caller can
		// tell it from a broken store without importing the store's error type.
		// Doing it here keeps the two entry points from drifting apart.
		//
		// It is also not logged as a store failure: an absent record is an answer,
		// and the callers that treat it as an error report it themselves.
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, domain.ErrProjectNotFound
		}
		slog.ErrorContext(ctx, "error getting project settings from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return entry, nil
}

func (s *NatsRepository) getProjectSettingsUnmarshal(ctx context.Context, entry jetstream.KeyValueEntry) (*models.ProjectSettings, error) {
	projectSettingsDB := models.ProjectSettings{}
	err := json.Unmarshal(entry.Value(), &projectSettingsDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project settings from NATS KV store", constants.ErrKey, err)
		return nil, err
	}

	return &projectSettingsDB, nil
}

// GetProjectSettings gets the project settings from the NATS KV store.
func (s *NatsRepository) GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error) {
	entry, err := s.getProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, err
	}

	return s.getProjectSettingsUnmarshal(ctx, entry)
}

// GetProjectSettingsWithRevision gets the project settings from the NATS KV store along with its revision.
func (s *NatsRepository) GetProjectSettingsWithRevision(ctx context.Context, projectUID string) (*models.ProjectSettings, uint64, error) {
	entry, err := s.getProjectSettings(ctx, projectUID)
	if err != nil {
		return nil, 0, err
	}

	projectSettingsDB, err := s.getProjectSettingsUnmarshal(ctx, entry)
	if err != nil {
		return nil, 0, err
	}

	return projectSettingsDB, entry.Revision(), nil
}

func (s *NatsRepository) updateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error {
	projectSettingsBytes, err := json.Marshal(projectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error marshalling project settings into JSON", constants.ErrKey, err)
		return err
	}

	_, err = s.ProjectSettings.Update(ctx, projectSettings.UID, projectSettingsBytes, revision)
	if err != nil {
		return err
	}

	return nil
}

// UpdateProjectSettings updates a project's settings information in the NATS KV store.
func (s *NatsRepository) UpdateProjectSettings(ctx context.Context, projectSettings *models.ProjectSettings, revision uint64) error {
	err := s.updateProjectSettings(ctx, projectSettings, revision)
	if err != nil {
		if isJetStreamCASConflict(err) {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	return nil
}

// deleteProjectSlugMapping removes the slug/<slug> index entry only when its stored value
// equals projectUID, using a LastRevision CAS delete. If the entry is missing or belongs to
// a different project the function logs and returns nil so the caller's work is not undone.
func (s *NatsRepository) deleteProjectSlugMapping(ctx context.Context, projectSlug, projectUID string) error {
	key := fmt.Sprintf(constants.KVSlugKey, projectSlug)
	entry, err := s.Projects.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		slog.ErrorContext(ctx, "error reading slug mapping before delete", constants.ErrKey, err)
		return err
	}

	if string(entry.Value()) != projectUID {
		slog.WarnContext(ctx, "slug mapping belongs to a different project; skipping delete",
			"slug", projectSlug, "expected_uid", projectUID, "actual_uid", string(entry.Value()))
		return nil
	}

	err = s.Projects.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		slog.ErrorContext(ctx, "error deleting slug mapping from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

func (s *NatsRepository) deleteProjectBase(ctx context.Context, projectUID string, revision uint64) error {
	err := s.Projects.Delete(ctx, projectUID, jetstream.LastRevision(revision))
	if err != nil {
		slog.ErrorContext(ctx, "error deleting project from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

func (s *NatsRepository) deleteProjectSettings(ctx context.Context, projectUID string) error {
	err := s.ProjectSettings.Delete(ctx, projectUID)
	if err != nil {
		slog.ErrorContext(ctx, "error deleting project settings from NATS KV store", constants.ErrKey, err)
		return err
	}

	return nil
}

// DeleteProject deletes a project from the NATS KV stores.
func (s *NatsRepository) DeleteProject(ctx context.Context, projectUID string, revision uint64) error {
	// Get the project to find the slug
	project, err := s.GetProjectBase(ctx, projectUID)
	if err != nil {
		return err
	}

	// Delete the project with revision check
	err = s.deleteProjectBase(ctx, projectUID, revision)
	if err != nil {
		if isJetStreamCASConflict(err) {
			slog.WarnContext(ctx, "revision mismatch", constants.ErrKey, err)
			return domain.ErrRevisionMismatch
		}
		return domain.ErrInternal
	}

	// Delete the slug mapping, verifying ownership before removing the index entry.
	err = s.deleteProjectSlugMapping(ctx, project.Slug, projectUID)
	if err != nil {
		return domain.ErrInternal
	}

	// Delete the project settings (if they exist)
	err = s.deleteProjectSettings(ctx, projectUID)
	if err != nil {
		return domain.ErrInternal
	}

	return nil
}
