// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the project service API that provides a RESTful API for managing projects
// and handles NATS messages for the project service.
package main

import (
	"context"
	_ "expvar"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/utils"
)

// Build-time variables set via ldflags
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

const (
	// gracefulShutdownSeconds should be higher than NATS client
	// request timeout, and lower than the pod or liveness probe's
	// terminationGracePeriodSeconds.
	gracefulShutdownSeconds = 25
)

func main() {
	os.Exit(run())
}

// run is the entry point for the service logic. It returns a non-zero exit
// code on startup failure so that orchestrators and CI can trigger
// crash-loop backoff or mark the pipeline step as failed.
// Deferred functions (OTel shutdown, context cancel) are called when run()
// returns, before os.Exit fires in main.
func run() int {
	env := parseEnv()
	flags := parseFlags(env.Port)

	log.InitStructureLogConfig()

	// Set up JWT validator needed by the [ProjectsService.JWTAuth] security handler.
	// This is initialized before OpenTelemetry so that a failure here does not
	// skip the deferred OTel shutdown. NewJWTAuth only stores config; actual
	// JWKS fetching happens at request time when OTel is active.
	jwtAuthConfig := auth.JWTAuthConfig{
		JWKSURL:            os.Getenv("JWKS_URL"),
		Audience:           os.Getenv("AUDIENCE"),
		MockLocalPrincipal: os.Getenv("JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL"),
	}
	jwtAuth, err := auth.NewJWTAuth(jwtAuthConfig)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("error setting up JWT authentication")
		return 1
	}

	// Set up OpenTelemetry SDK.
	// Command-line/environment OTEL_SERVICE_VERSION takes precedence over
	// the build-time Version variable.
	otelConfig := utils.OTelConfigFromEnv()
	if otelConfig.ServiceVersion == "" {
		otelConfig.ServiceVersion = Version
	}
	otelShutdown, err := utils.SetupOTelSDKWithConfig(context.Background(), otelConfig)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("error setting up OpenTelemetry SDK")
		return 1
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()
		if shutdownErr := otelShutdown(ctx); shutdownErr != nil {
			slog.With(constants.ErrKey, shutdownErr).Error("error shutting down OpenTelemetry SDK")
		}
	}()

	gracefulCloseWG := sync.WaitGroup{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Connect to NATS and build all infrastructure dependencies before constructing
	// the service, so the service is fully valid from the moment it is created.
	natsConn, deps, err := setupNATS(ctx, env, &gracefulCloseWG, done)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("error setting up NATS")
		// setupNATS may return a live connection alongside the error (e.g. when
		// KV store creation fails after connect succeeds). Drain it so the
		// server-side subscriptions are cleanly closed before we exit.
		if natsConn != nil {
			_ = natsConn.Drain()
		}
		return 1
	}

	// Construct a fully valid service with all dependencies wired at once.
	projectService := service.NewProjectsService(jwtAuth, service.ServiceConfig{
		SkipEtagValidation:  env.SkipEtagValidation,
		LFXSelfServeBaseURL: env.LFXSelfServeBaseURL,
		EmailsEnabled:       env.EmailsEnabled,
		InvitesEnabled:      env.InvitesEnabled,
	}, deps)
	svc := NewProjectsAPI(projectService)

	// Wire NATS event and RPC subscriptions now that the service is ready.
	if err := createNatsSubcriptions(ctx, svc, natsConn); err != nil {
		slog.With(constants.ErrKey, err).Error("error creating NATS subscriptions")
		return 1
	}

	// Start the HTTP server now that the service is fully initialised.
	httpServer := setupHTTPServer(flags, svc, &gracefulCloseWG)

	// Block until SIGINT or SIGTERM is received.
	<-done

	gracefulShutdown(httpServer, natsConn, &gracefulCloseWG, cancel)
	return 0
}
