// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package service contains the Goa service implementation and provider
// initialization for the member API.
package service

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	sf "github.com/k-capehart/go-salesforce/v3"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	infraproject "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/project"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce"
)

var (
	natsClient *nats.NATSClient
	natsDoOnce sync.Once

	sfClient *sf.Salesforce
	sfDoOnce sync.Once

	projectResolver port.ProjectResolver
	resolverDoOnce  sync.Once
)

// natsTimeoutFromEnv reads the NATS_TIMEOUT environment variable and returns it
// as a time.Duration. Defaults to 10 seconds if unset. Calls log.Fatalf on an
// unparseable value.
func natsTimeoutFromEnv() time.Duration {
	raw := os.Getenv("NATS_TIMEOUT")
	if raw == "" {
		raw = "10s"
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("invalid NATS timeout duration %q: %v", raw, err)
	}
	return d
}

func natsInit(ctx context.Context) {
	natsDoOnce.Do(func() {
		natsURL := os.Getenv("NATS_URL")
		if natsURL == "" {
			natsURL = "nats://localhost:4222"
		}

		natsMaxReconnect := os.Getenv("NATS_MAX_RECONNECT")
		if natsMaxReconnect == "" {
			natsMaxReconnect = "3"
		}
		natsMaxReconnectInt, err := strconv.Atoi(natsMaxReconnect)
		if err != nil {
			log.Fatalf("invalid NATS max reconnect value %s: %v", natsMaxReconnect, err)
		}

		natsReconnectWait := os.Getenv("NATS_RECONNECT_WAIT")
		if natsReconnectWait == "" {
			natsReconnectWait = "2s"
		}
		natsReconnectWaitDuration, err := time.ParseDuration(natsReconnectWait)
		if err != nil {
			log.Fatalf("invalid NATS reconnect wait duration %s: %v", natsReconnectWait, err)
		}

		config := nats.Config{
			URL:           natsURL,
			Timeout:       natsTimeoutFromEnv(),
			MaxReconnect:  natsMaxReconnectInt,
			ReconnectWait: natsReconnectWaitDuration,
		}

		client, err := nats.NewClient(ctx, config)
		if err != nil {
			log.Fatalf("failed to create NATS client: %v", err)
		}
		natsClient = client
	})
}

func sfInit(ctx context.Context) {
	sfDoOnce.Do(func() {
		cfg, err := salesforce.ConfigFromEnv()
		if err != nil {
			log.Fatalf("failed to read Salesforce config from environment: %v", err)
		}

		client, err := cfg.Init()
		if err != nil {
			log.Fatalf("failed to authenticate with Salesforce: %v", err)
		}
		sfClient = client
		slog.InfoContext(ctx, "Salesforce client initialised")
	})
}

// CloseNATSClient closes the NATS client connection if it was initialised.
func CloseNATSClient() {
	if natsClient != nil {
		natsClient.Close() //nolint:errcheck // NATS Close does not return a meaningful error in practice.
	}
}

// NATSClientImpl returns the shared NATSClient singleton, initialising it if
// necessary. This is intended for use by main.go to register NATS RPC
// subscriptions after MemberReaderImpl has been called.
func NATSClientImpl(ctx context.Context) *nats.NATSClient {
	natsInit(ctx)
	return natsClient
}

// ProjectResolverImpl returns the shared ProjectResolver singleton, initialising
// all dependencies (NATS, Salesforce) as needed. Returns nil when
// REPOSITORY_SOURCE=mock — callers must guard on nil before use.
func ProjectResolverImpl(ctx context.Context) port.ProjectResolver {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		return nil

	case "salesforce":
		resolverDoOnce.Do(func() {
			natsInit(ctx)
			sfInit(ctx)

			cache := nats.NewStorage(natsClient)
			projectRepo := salesforce.NewProjectRepo(sfClient)
			projectRPC := nats.NewProjectRPC(natsClient.Conn(), natsTimeoutFromEnv())
			projectResolver = infraproject.NewProjectResolver(projectRPC, projectRepo, cache)
		})
		return projectResolver

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// KeyContactWriterImpl initialises and returns the port.KeyContactWriter
// implementation selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — Salesforce-backed writer with NATS KV cache invalidation.
//   - "mock"                 — Stub that returns NotImplemented for all writes; for local development without SF credentials.
func KeyContactWriterImpl(ctx context.Context) port.KeyContactWriter {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock key contact writer")
		return mock.NewMockKeyContactWriter()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce key contact writer with NATS KV cache")
		natsInit(ctx)
		sfInit(ctx)
		cache := nats.NewStorage(natsClient)
		contactRepo := salesforce.NewContactRepo(sfClient)
		contactsRepo := salesforce.NewKeyContactRepo(sfClient)
		return salesforce.NewKeyContactWriter(sfClient, contactsRepo, contactRepo, cache)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// MemberReaderImpl initialises and returns the port.MemberReader implementation
// selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — Salesforce SOQL queries with NATS KV caching.
//   - "mock"                 — In-memory mock, for local development only.
func MemberReaderImpl(ctx context.Context) port.MemberReader {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock member reader")
		return mock.NewMockMembershipRepository()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce member reader with NATS KV cache")

		natsInit(ctx)
		sfInit(ctx)

		// ProjectResolverImpl shares the singleton resolver so that the KV
		// cache entries written by the resolver during list/get calls are also
		// available to the NATS RPC handler registered in main.go.
		resolver := ProjectResolverImpl(ctx)
		cache := nats.NewStorage(natsClient)

		return salesforce.NewMemberReader(
			salesforce.NewMemberRepo(sfClient),
			salesforce.NewMembershipRepo(sfClient),
			salesforce.NewKeyContactRepo(sfClient),
			resolver,
			cache,
		)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// B2BOrgReaderImpl initialises and returns the port.B2BOrgReader implementation
// selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — Salesforce SOQL queries with NATS KV caching.
//   - "mock"                 — In-memory mock that always returns empty pages.
func B2BOrgReaderImpl(ctx context.Context) port.B2BOrgReader {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock B2B org reader")
		return mock.NewMockB2BOrgReader()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce B2B org reader with NATS KV cache")

		natsInit(ctx)
		sfInit(ctx)

		cache := nats.NewStorage(natsClient)
		return salesforce.NewB2BOrgReader(
			salesforce.NewAccountRepo(sfClient),
			cache,
		)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}
