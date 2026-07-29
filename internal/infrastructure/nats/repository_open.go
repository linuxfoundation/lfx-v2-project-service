// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go/jetstream"
)

// OpenRepository opens all NATS KV and object stores used by the project repository.
func OpenRepository(ctx context.Context, js jetstream.JetStream) (*NatsRepository, error) {
	repo := &NatsRepository{}

	projectsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjects)
	if err != nil {
		return nil, fmt.Errorf("open KV store %q: %w", constants.KVStoreNameProjects, err)
	}
	repo.Projects = projectsKV

	projectSettingsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectSettings)
	if err != nil {
		return nil, fmt.Errorf("open KV store %q: %w", constants.KVStoreNameProjectSettings, err)
	}
	repo.ProjectSettings = projectSettingsKV

	linksKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectLinks)
	if err != nil {
		return nil, fmt.Errorf("open KV store %q: %w", constants.KVStoreNameProjectLinks, err)
	}
	repo.Links = linksKV

	foldersKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectFolders)
	if err != nil {
		return nil, fmt.Errorf("open KV store %q: %w", constants.KVStoreNameProjectFolders, err)
	}
	repo.Folders = foldersKV

	documentsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectDocuments)
	if err != nil {
		return nil, fmt.Errorf("open KV store %q: %w", constants.KVStoreNameProjectDocuments, err)
	}
	repo.Documents = documentsKV

	documentFiles, err := js.ObjectStore(ctx, constants.ObjectStoreNameProjectDocuments)
	if err != nil {
		return nil, fmt.Errorf("open object store %q: %w", constants.ObjectStoreNameProjectDocuments, err)
	}
	repo.DocumentFiles = documentFiles

	return repo, nil
}
