// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is a utility that ensures a root project exists in the project service
// key-value store for teams permissions assignment.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	natsio "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	errKey              = "error"
	rootProjectSlug     = "ROOT"
	rootProjectDesc     = "A root project for teams permissions assignment, ordinarily hidden from users."
	rootProjectSlugKey  = "slug/ROOT"
	gracefulShutdownSec = 25
)

func main() {
	os.Exit(run())
}

func run() int {
	env := parseEnv()
	log.InitStructureLogConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := setupRootProject(ctx, env); err != nil {
		slog.With(errKey, err).Error("failed to setup root project")
		return 1
	}

	slog.Info("root project setup completed successfully")
	return 0
}

type environment struct {
	NatsURL             string
	RootProjectWriters  []models.UserInfo
	RootProjectAuditors []models.UserInfo
}

func parseEnv() environment {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// Parse comma-separated writers and auditors
	writersStr := os.Getenv("ROOT_PROJECT_WRITERS")
	auditorsStr := os.Getenv("ROOT_PROJECT_AUDITORS")

	writers := parseUserInfo(writersStr)
	auditors := parseUserInfo(auditorsStr)

	return environment{
		NatsURL:             natsURL,
		RootProjectWriters:  writers,
		RootProjectAuditors: auditors,
	}
}

// parseUserInfo parses a comma-separated string of user information.
// Supports two formats:
// 1. Simple: "username1,username2" (backward compatibility)
// 2. Structured: "username:name:email:avatar,username2:name2:email2:avatar2"
func parseUserInfo(userStr string) []models.UserInfo {
	if userStr == "" {
		return nil
	}

	var users []models.UserInfo
	for _, userEntry := range strings.Split(userStr, ",") {
		if trimmed := strings.TrimSpace(userEntry); trimmed != "" {
			// Check if this is structured format (contains colons)
			if strings.Contains(trimmed, ":") {
				// Structured format: username:name:email:avatar
				// Use SplitN to limit splitting to 4 parts, preserving colons in URLs (e.g., https://)
				parts := strings.SplitN(trimmed, ":", 4)
				user := models.UserInfo{
					Username: parts[0],
				}
				if len(parts) > 1 {
					user.Name = parts[1]
				}
				if len(parts) > 2 {
					user.Email = parts[2]
				}
				if len(parts) > 3 {
					user.Avatar = parts[3]
				}
				users = append(users, user)
			} else {
				// Simple format: just username (backward compatibility)
				users = append(users, models.UserInfo{
					Username: trimmed,
					Name:     trimmed, // Use username as name for backward compatibility
					Email:    "",
					Avatar:   "",
				})
			}
		}
	}
	return users
}

func setupRootProject(ctx context.Context, env environment) error {
	// Connect to NATS
	slog.With("nats_url", env.NatsURL).Info("connecting to NATS")
	natsConn, err := natsio.Connect(
		env.NatsURL,
		natsio.DrainTimeout(gracefulShutdownSec*time.Second),
		natsio.ConnectHandler(func(_ *natsio.Conn) {
			slog.With("nats_url", env.NatsURL).Info("NATS connection established")
		}),
		natsio.ErrorHandler(func(_ *natsio.Conn, s *natsio.Subscription, err error) {
			if s != nil {
				slog.With(errKey, err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				slog.With(errKey, err).Error("async NATS error outside subscription")
			}
		}),
	)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	// Get the key-value store for projects
	kv, err := getKeyValueStore(ctx, natsConn)
	if err != nil {
		return err
	}

	// Check if ROOT project already exists
	if p, err := getRootProject(ctx, kv.Projects); err != nil {
		return err
	} else if p != nil {
		slog.With("project", p).Info("ROOT project already exists, nothing to do")
		return nil
	}

	// Create the ROOT project
	return createRootProject(ctx, kv, env, natsConn)
}

type kvBuckets struct {
	Projects        jetstream.KeyValue
	ProjectSettings jetstream.KeyValue
}

func getKeyValueStore(ctx context.Context, natsConn *natsio.Conn) (kvBuckets, error) {
	kv := kvBuckets{}

	js, err := jetstream.New(natsConn)
	if err != nil {
		slog.ErrorContext(ctx, "error creating NATS JetStream client", "nats_url", natsConn.ConnectedUrl(), errKey, err)
		return kv, err
	}
	projectsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjects)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "bucket", constants.KVStoreNameProjects)
		return kv, err
	}
	projectSettingsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "bucket", constants.KVStoreNameProjectSettings)
		return kv, err
	}
	kv.Projects = projectsKV
	kv.ProjectSettings = projectSettingsKV
	return kv, nil
}

func getRootProject(ctx context.Context, projectsKV jetstream.KeyValue) (*models.ProjectBase, error) {
	// Try to get the ROOT project by slug
	uidEntry, err := projectsKV.Get(ctx, rootProjectSlugKey)
	if err != nil {
		// The root project not existing isn't an error we care about, it just means we need to create it.
		if err == jetstream.ErrKeyNotFound {
			return nil, nil
		}
		slog.ErrorContext(ctx, "error checking for ROOT project existence", errKey, err)
		return nil, err
	}

	// Try to get the ROOT project by UID from slug -> UID mapping
	p, err := projectsKV.Get(ctx, string(uidEntry.Value()))
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			slog.ErrorContext(ctx, "ROOT project UID not found in KV store but slug key was", errKey, err)
			return nil, errors.New("ROOT project UID not found in KV store")
		}
		slog.ErrorContext(ctx, "error checking for ROOT project existence by UID", errKey, err)
		return nil, err
	}

	var projectDB *models.ProjectBase
	err = json.Unmarshal(p.Value(), &projectDB)
	if err != nil {
		slog.ErrorContext(ctx, "error unmarshalling project from NATS KV store", errKey, err, "project_id", uidEntry.Value())
		return nil, err
	}

	return projectDB, nil
}

func createRootProject(ctx context.Context, kv kvBuckets, env environment, natsConn *natsio.Conn) error {
	currentTimeFmt := time.Now().UTC()
	rootProject := models.ProjectBase{
		UID:         uuid.New().String(),
		Slug:        rootProjectSlug,
		Name:        rootProjectSlug,
		Description: rootProjectDesc,
		Public:      false,
		ParentUID:   "",
		CreatedAt:   &currentTimeFmt,
		UpdatedAt:   &currentTimeFmt,
	}
	rootProjectSettings := models.ProjectSettings{
		UID:              rootProject.UID,
		MissionStatement: rootProjectDesc,
		Writers:          env.RootProjectWriters,
		Auditors:         env.RootProjectAuditors,
		CreatedAt:        &currentTimeFmt,
		UpdatedAt:        &currentTimeFmt,
	}

	projectJSON, err := json.Marshal(rootProject)
	if err != nil {
		slog.ErrorContext(ctx, "error marshaling ROOT project", errKey, err)
		return err
	}

	projectSettingsJSON, err := json.Marshal(rootProjectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error marshaling ROOT project settings", errKey, err)
		return err
	}

	// insert slug/ROOT -> UID mapping
	_, err = kv.Projects.Put(ctx, rootProjectSlugKey, []byte(rootProject.UID))
	if err != nil {
		slog.ErrorContext(ctx, "error storing ROOT project in KV store", errKey, err)
		return err
	}

	// insert project base via UID
	_, err = kv.Projects.Put(ctx, string(rootProject.UID), projectJSON)
	if err != nil {
		slog.ErrorContext(ctx, "error storing ROOT project in KV store", errKey, err)
		return err
	}

	// insert project settings via UID
	_, err = kv.ProjectSettings.Put(ctx, string(rootProject.UID), projectSettingsJSON)
	if err != nil {
		slog.ErrorContext(ctx, "error storing ROOT project settings in KV store", errKey, err)
		return err
	}

	slog.With("uid", rootProject.UID, "slug", rootProject.Slug, "writers", env.RootProjectWriters, "auditors", env.RootProjectAuditors).Info("ROOT project created successfully")

	// Send index message for the newly created root project
	if err := sendIndexMessage(ctx, natsConn, rootProject, rootProjectSettings); err != nil {
		slog.With("error", err).Error("failed to send index message for ROOT project")
		return err
	}

	return nil
}

func sendIndexMessage(ctx context.Context, natsConn *natsio.Conn, project models.ProjectBase, settings models.ProjectSettings) error {
	// Create message builder using existing infrastructure
	msgBuilder := &nats.MessageBuilder{
		NatsConn: natsConn,
	}

	// Create and send the project indexer message
	projectMessage := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionCreated,
		Data:           project,
		Tags:           []string{}, // Empty tags for root project
		IndexingConfig: project.IndexingConfig(),
	}

	if err := msgBuilder.SendIndexerMessage(ctx, constants.IndexProjectSubject, projectMessage, false); err != nil {
		slog.ErrorContext(ctx, "error sending project index message", errKey, err)
		return err
	}

	// Create and send the project settings indexer message
	settingsMessage := indexerTypes.IndexerMessageEnvelope{
		Action:         indexerConstants.ActionCreated,
		Data:           settings,
		Tags:           []string{}, // Empty tags for root project
		IndexingConfig: settings.IndexingConfig(project.UID),
	}

	if err := msgBuilder.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, settingsMessage, false); err != nil {
		slog.ErrorContext(ctx, "error sending project settings index message", errKey, err)
		return err
	}

	slog.DebugContext(ctx, "successfully sent index messages for ROOT project")
	return nil
}
