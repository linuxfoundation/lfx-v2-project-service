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

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/auth"
	infrab2borg "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/b2borg"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/mock"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	infraproject "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/project"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce"
	usecaseSvc "github.com/linuxfoundation/lfx-v2-member-service/internal/service"
)

var (
	natsClient *nats.NATSClient
	natsDoOnce sync.Once

	sfClient *sf.Salesforce
	sfDoOnce sync.Once

	sObjectClient *salesforce.SObjectClient
	sObjectDoOnce sync.Once

	projectResolver port.ProjectResolver
	resolverDoOnce  sync.Once

	// mockSettings is the shared in-memory settings store used in mock mode.
	// Reader and writer must point at the same instance so writes are visible to reads.
	mockSettings     *mock.MockB2BOrgSettings
	mockSettingsOnce sync.Once
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

func sObjectClientInit(ctx context.Context) {
	sObjectDoOnce.Do(func() {
		natsInit(ctx)
		sfInit(ctx)
		sObjectCache := nats.NewSObjectCache(natsClient)
		sObjectClient = salesforce.NewSObjectClient(sfClient, sObjectCache)
		slog.InfoContext(ctx, "Salesforce sObject client initialised")
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
		slog.InfoContext(ctx, "initialising Salesforce key contact writer with conditional writes")
		sObjectClientInit(ctx)
		cache := nats.NewStorage(natsClient)
		contactRepo := salesforce.NewContactRepo(sfClient)
		contactsRepo := salesforce.NewKeyContactRepo(sfClient)
		return salesforce.NewKeyContactWriter(sfClient, sObjectClient, contactsRepo, contactRepo, cache)

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

		sObjectClientInit(ctx)
		return salesforce.NewMemberReader(
			salesforce.NewMemberRepo(sfClient),
			salesforce.NewMembershipRepo(sfClient),
			salesforce.NewKeyContactRepo(sfClient),
			salesforce.NewKeyContactReader(sObjectClient),
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
		slog.InfoContext(ctx, "initialising Salesforce B2B org reader with sObject cache")
		sObjectClientInit(ctx) // also calls sfInit internally
		return salesforce.NewB2BOrgReader(sObjectClient, sfClient)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// B2BOrgWriterImpl initialises and returns the port.B2BOrgWriter implementation
// selected by the REPOSITORY_SOURCE environment variable.
func B2BOrgWriterImpl(ctx context.Context) port.B2BOrgWriter {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock B2B org writer")
		return mock.NewMockB2BOrgWriter()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce B2B org writer with sObject cache")
		sObjectClientInit(ctx)
		return salesforce.NewB2BOrgWriter(sObjectClient)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// ProjectMembershipReaderImpl initialises and returns the port.ProjectMembershipReader
// implementation selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — Salesforce sObject REST API reader.
//   - "mock"                 — Stub that always returns not-found; for local development without SF credentials.
func ProjectMembershipReaderImpl(ctx context.Context) port.ProjectMembershipReader {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock project membership reader")
		return mock.NewMockProjectMembershipReader()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce project membership reader with sObject cache")
		sObjectClientInit(ctx)
		return salesforce.NewProjectMembershipReader(sObjectClient)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// MemberPublisherImpl initialises and returns the port.MemberPublisher
// implementation selected by the MESSAGING_SOURCE environment variable:
//
//   - "nats" (default) — NATS JetStream publisher.
//   - "mock"           — No-op publisher that logs published messages.
//
// When MESSAGING_SOURCE=mock and GLOBAL_ORG_ADMIN_TEAM_UID is empty, the
// service still starts successfully — useful for local development.
func MemberPublisherImpl(ctx context.Context) port.MemberPublisher {
	msgSource := os.Getenv("MESSAGING_SOURCE")
	if msgSource == "" {
		msgSource = "nats"
	}

	switch msgSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock member publisher")
		return mock.NewMockMemberPublisher()

	case "nats":
		slog.InfoContext(ctx, "initialising NATS member publisher")
		natsInit(ctx)
		return nats.NewMessagePublisher(natsClient)

	default:
		log.Fatalf("unsupported MESSAGING_SOURCE value: %q", msgSource)
		return nil
	}
}

// UserReaderImpl returns the port.UserReader implementation selected by the
// REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — NATS RPC to auth-service.
//   - "mock"                 — No-op that always returns empty sub.
func UserReaderImpl(ctx context.Context) port.UserReader {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock user reader")
		return mock.NewMockUserReader()

	case "salesforce":
		slog.InfoContext(ctx, "initialising NATS user reader (auth-service)")
		natsInit(ctx)
		return nats.NewUserReader(natsClient)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// GlobalOrgAdminTeamUID reads the GLOBAL_ORG_ADMIN_TEAM_UID environment variable.
// Returns empty string when not set (allowed in mock/messaging=mock mode; the
// FGA message simply omits the global_org_admin reference).
func GlobalOrgAdminTeamUID() string {
	return os.Getenv("GLOBAL_ORG_ADMIN_TEAM_UID")
}

// BackfillIteratorImpl returns the BackfillIterator implementation selected by
// the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — Salesforce SOQL paged iterators.
//   - "mock"                 — In-memory mock with no pre-loaded pages.
func BackfillIteratorImpl(ctx context.Context) usecaseSvc.BackfillIterator {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock backfill iterator")
		return &mock.MockBackfillIterator{}

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce backfill iterator")
		sfInit(ctx)
		return salesforce.NewBackfillIterator(
			salesforce.NewAccountRepo(sfClient),
			salesforce.NewMembershipRepo(sfClient),
			salesforce.NewKeyContactRepo(sfClient),
		)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
		return nil
	}
}

// mockSettingsInstance returns the shared MockB2BOrgSettings singleton.
// Reader and writer must share the same instance so writes are visible to reads.
func mockSettingsInstance(ctx context.Context) *mock.MockB2BOrgSettings {
	mockSettingsOnce.Do(func() {
		slog.InfoContext(ctx, "initialising mock B2B org settings store")
		mockSettings = mock.NewMockB2BOrgSettings()
	})
	return mockSettings
}

// B2BOrgSettingsReaderImpl returns the port.B2BOrgSettingsReader implementation
// selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — NATS KV "org-settings" bucket (authoritative, no MaxAge TTL).
//   - "mock"                 — Shared in-memory mock; lets the service start without NATS.
func B2BOrgSettingsReaderImpl(ctx context.Context) port.B2BOrgSettingsReader {
	if os.Getenv("REPOSITORY_SOURCE") == "mock" {
		return mockSettingsInstance(ctx)
	}
	natsInit(ctx)
	return nats.NewStorage(natsClient)
}

// B2BOrgSettingsWriterImpl returns the port.B2BOrgSettingsWriter implementation
// selected by the REPOSITORY_SOURCE environment variable:
//
//   - "salesforce" (default) — NATS KV "org-settings" bucket.
//   - "mock"                 — Shared in-memory mock; same instance as B2BOrgSettingsReaderImpl.
func B2BOrgSettingsWriterImpl(ctx context.Context) port.B2BOrgSettingsWriter {
	if os.Getenv("REPOSITORY_SOURCE") == "mock" {
		return mockSettingsInstance(ctx)
	}
	natsInit(ctx)
	return nats.NewStorage(natsClient)
}

// BackfillRunnerImpl constructs a BackfillRunner wired with all production
// (or mock) dependencies based on REPOSITORY_SOURCE / MESSAGING_SOURCE.
//
// In mock mode natsClient is nil — the runner skips the distributed full-run
// lock (safe for local development; no concurrent runners exist).
func BackfillRunnerImpl(ctx context.Context) *usecaseSvc.Runner {
	repoSource := os.Getenv("REPOSITORY_SOURCE")
	if repoSource == "" {
		repoSource = "salesforce"
	}

	var kcReader usecaseSvc.KeyContactSObjectReader
	var nc *nats.NATSClient

	switch repoSource {
	case "mock":
		slog.InfoContext(ctx, "initialising mock backfill runner")
		kcReader = mock.NewMockKeyContactSObjectReader()

	case "salesforce":
		slog.InfoContext(ctx, "initialising Salesforce backfill runner")
		natsInit(ctx)
		nc = natsClient
		sObjectClientInit(ctx)
		kcReader = salesforce.NewKeyContactReader(sObjectClient)

	default:
		log.Fatalf("unsupported REPOSITORY_SOURCE value: %q", repoSource)
	}

	return usecaseSvc.NewRunner(
		BackfillIteratorImpl(ctx),
		B2BOrgReaderImpl(ctx),
		ProjectMembershipReaderImpl(ctx),
		kcReader,
		B2BOrgSettingsReaderImpl(ctx),
		MemberPublisherImpl(ctx),
		nc,
	)
}

// JWTAuthImpl constructs the domain.Authenticator from environment variables.
// Calls log.Fatalf on configuration or key-fetch errors — same fail-fast
// pattern as the other provider functions.
func JWTAuthImpl(ctx context.Context) domain.Authenticator {
	cfg := auth.JWTAuthConfig{
		JWKSURL:            os.Getenv("JWKS_URL"),
		Audience:           os.Getenv("AUDIENCE"),
		MockLocalPrincipal: os.Getenv("JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL"),
	}
	a, err := auth.NewJWTAuth(cfg)
	if err != nil {
		log.Fatalf("failed to set up JWT authentication: %v", err)
	}
	return a
}

// MemberReaderUseCase constructs the MemberReaderOrchestrator use-case wired
// with the production (or mock) MemberReader adapter.
func MemberReaderUseCase(ctx context.Context) usecaseSvc.MemberReader {
	return usecaseSvc.NewMemberReaderOrchestrator(
		usecaseSvc.WithMemberReader(MemberReaderImpl(ctx)),
	)
}

// B2BOrgWriterUseCase constructs the B2BOrgWriter use-case orchestrator wired
// with all production (or mock) dependencies.
func B2BOrgWriterUseCase(ctx context.Context) usecaseSvc.B2BOrgWriter {
	return usecaseSvc.NewB2BOrgWriter(
		usecaseSvc.WithB2BOrgReader(B2BOrgReaderImpl(ctx)),
		usecaseSvc.WithB2BOrgWriter(B2BOrgWriterImpl(ctx)),
		usecaseSvc.WithB2BOrgPublisher(MemberPublisherImpl(ctx)),
		usecaseSvc.WithGlobalOrgAdminTeamUID(GlobalOrgAdminTeamUID()),
	)
}

// KeyContactWriterUseCase constructs the KeyContactWriter use-case orchestrator.
func KeyContactWriterUseCase(ctx context.Context) usecaseSvc.KeyContactWriter {
	return usecaseSvc.NewKeyContactWriter(
		usecaseSvc.WithKCStorage(MemberReaderImpl(ctx)),
		usecaseSvc.WithKCWriter(KeyContactWriterImpl(ctx)),
		usecaseSvc.WithKCProjectMembershipReader(ProjectMembershipReaderImpl(ctx)),
		usecaseSvc.WithKCPublisher(MemberPublisherImpl(ctx)),
		usecaseSvc.WithKCUserReader(UserReaderImpl(ctx)),
	)
}

// OrgSettingsWriterUseCase constructs the OrgSettingsWriter use-case orchestrator.
func OrgSettingsWriterUseCase(ctx context.Context) usecaseSvc.OrgSettingsWriter {
	return usecaseSvc.NewOrgSettingsWriter(
		usecaseSvc.WithOrgSettingsReader(B2BOrgSettingsReaderImpl(ctx)),
		usecaseSvc.WithOrgSettingsWriter(B2BOrgSettingsWriterImpl(ctx)),
		usecaseSvc.WithOrgSettingsB2BOrgReader(B2BOrgReaderImpl(ctx)),
		usecaseSvc.WithOrgSettingsPublisher(MemberPublisherImpl(ctx)),
	)
}

// B2BOrgResolverImpl returns a B2BOrgResolver that translates Salesforce Account
// SFIDs to v2 b2b_org UUIDs via a deterministic base-62 transform (no I/O).
// Unlike other providers, there is no mock/salesforce distinction — the resolver
// is pure CPU and works identically in every mode.
func B2BOrgResolverImpl(_ context.Context) port.B2BOrgResolver {
	return infrab2borg.NewResolver()
}
