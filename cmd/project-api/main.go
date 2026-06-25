// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the project service API that provides a RESTful API for managing projects
// and handles NATS messages for the project service.
package main

import (
	"context"
	_ "expvar"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	goahttp "goa.design/goa/v3/http"

	inviteapi "github.com/linuxfoundation/lfx-v2-invite-service/pkg/api"
	genhttp "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	genquerysvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/auth"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/middleware"
	internalnats "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
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
	// errKey is the key for the error field in the slog.
	errKey = "error"
	// gracefulShutdownSeconds should be higher than NATS client
	// request timeout, and lower than the pod or liveness probe's
	// terminationGracePeriodSeconds.
	gracefulShutdownSeconds = 25
)

func main() {
	env := parseEnv()
	flags := parseFlags(env.Port)

	log.InitStructureLogConfig()

	// Set up JWT validator needed by the [ProjectsService.JWTAuth] security handler.
	// This is initialized before OpenTelemetry so that os.Exit(1) does not
	// skip the deferred OTel shutdown. NewJWTAuth only stores config; actual
	// JWKS fetching happens at request time when OTel is active.
	jwtAuthConfig := auth.JWTAuthConfig{
		JWKSURL:            os.Getenv("JWKS_URL"),
		Audience:           os.Getenv("AUDIENCE"),
		MockLocalPrincipal: os.Getenv("JWT_AUTH_DISABLED_MOCK_LOCAL_PRINCIPAL"),
	}
	jwtAuth, err := auth.NewJWTAuth(jwtAuthConfig)
	if err != nil {
		slog.With(errKey, err).Error("error setting up JWT authentication")
		os.Exit(1)
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
		slog.With(errKey, err).Error("error setting up OpenTelemetry SDK")
		os.Exit(1)
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()
		if shutdownErr := otelShutdown(ctx); shutdownErr != nil {
			slog.With(errKey, shutdownErr).Error("error shutting down OpenTelemetry SDK")
		}
	}()

	// Generated service initialization.
	service := service.NewProjectsService(jwtAuth, service.ServiceConfig{
		SkipEtagValidation:  env.SkipEtagValidation,
		LFXSelfServeBaseURL: env.LFXSelfServeBaseURL,
		EmailsEnabled:       env.EmailsEnabled,
		InvitesEnabled:      env.InvitesEnabled,
	})
	svc := NewProjectsAPI(service)

	gracefulCloseWG := sync.WaitGroup{}

	httpServer := setupHTTPServer(flags, svc, &gracefulCloseWG)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	natsConn, err := setupNATS(ctx, env, svc, &gracefulCloseWG, done)
	if err != nil {
		slog.With(errKey, err).Error("error setting up NATS")
		return
	}

	// This next line blocks until SIGINT or SIGTERM is received.
	<-done

	gracefulShutdown(httpServer, natsConn, &gracefulCloseWG, cancel)

}

// flags are the command line flags for the project service.
type flags struct {
	Debug bool
	Port  string
	Bind  string
}

func parseFlags(defaultPort string) flags {
	var debug = flag.Bool("d", false, "enable debug logging")
	var port = flag.String("p", defaultPort, "listen port")
	var bind = flag.String("bind", "*", "interface to bind on")

	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	// Based on the debug flag, set the log level environment variable used by [log.InitStructureLogConfig]
	if *debug {
		err := os.Setenv("LOG_LEVEL", "debug")
		if err != nil {
			slog.With(errKey, err).Error("error setting log level")
			os.Exit(1)
		}
	}

	return flags{
		Debug: *debug,
		Port:  *port,
		Bind:  *bind,
	}
}

// environment are the environment variables for the project service.
type environment struct {
	NatsURL             string
	Port                string
	SkipEtagValidation  bool
	LFXSelfServeBaseURL string
	EmailsEnabled       bool
	InvitesEnabled      bool
}

func parseEnv() environment {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	skipEtagValidation := false
	skipEtagValidationStr := os.Getenv("SKIP_ETAG_VALIDATION")
	if skipEtagValidationStr == "true" {
		skipEtagValidation = true
	}
	lfxSelfServeBaseURL := LFXSelfServeBaseURL()
	return environment{
		NatsURL:             natsURL,
		Port:                port,
		SkipEtagValidation:  skipEtagValidation,
		LFXSelfServeBaseURL: lfxSelfServeBaseURL,
		EmailsEnabled:       os.Getenv("EMAILS_ENABLED") == "true",
		InvitesEnabled:      os.Getenv("INVITES_ENABLED") == "true",
	}
}

// LFXSelfServeBaseURL derives the LFX Self-Serve base URL from environment variables.
// LFX_SELF_SERVE_BASE_URL takes precedence; otherwise it falls back to LFX_ENVIRONMENT.
// When LFX_ENVIRONMENT is unset or unrecognized, prod is assumed (safe default for deployed environments).
func LFXSelfServeBaseURL() string {
	if url := os.Getenv("LFX_SELF_SERVE_BASE_URL"); url != "" {
		return url
	}
	switch os.Getenv("LFX_ENVIRONMENT") {
	case "prod", "production":
		return "https://app.lfx.dev"
	case "staging", "stg", "stage":
		return "https://app.staging.lfx.dev"
	case "dev", "development":
		return "https://app.dev.lfx.dev"
	default:
		return "https://app.lfx.dev"
	}
}

func setupHTTPServer(flags flags, svc *ProjectsAPI, gracefulCloseWG *sync.WaitGroup) *http.Server {
	// Wrap it in the generated endpoints
	endpoints := genquerysvc.NewEndpoints(svc)

	// Build an HTTP handler
	mux := goahttp.NewMuxer()
	requestDecoder := goahttp.RequestDecoder
	responseEncoder := goahttp.ResponseEncoder

	// Create a custom encoder that sets ETag header for get-one-project
	customEncoder := func(ctx context.Context, w http.ResponseWriter) goahttp.Encoder {
		encoder := responseEncoder(ctx, w)

		// Check if we have an ETag in the context
		if etag, ok := ctx.Value(constants.ETagContextID).(string); ok {
			w.Header().Set("ETag", etag)
		}

		return encoder
	}

	koDataPath := os.Getenv("KO_DATA_PATH")
	if koDataPath == "" {
		koDataPath = "./api/project/v1/gen/http/"
	}

	koDataDir := http.Dir(koDataPath)

	genHttpServer := genhttp.New(
		endpoints,
		mux,
		requestDecoder,
		customEncoder,
		nil,
		nil,
		uploadDocumentDecoder,
		koDataDir,
		koDataDir,
		koDataDir,
		koDataDir,
	)

	// Register route-tagging middleware inside chi's routing chain so that
	// http.route is set on the OTel span after chi has matched the route pattern.
	// The span name is also updated here to avoid high-cardinality names from
	// using raw URL paths (which contain actual path parameter values).
	// Must be registered before Mount calls per chi convention.
	// Reads RoutePattern after next.ServeHTTP because chi populates the pattern
	// during routing (inside ServeHTTP), not before.
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rctx := chi.RouteContext(r.Context())
				if rctx != nil {
					routePattern := rctx.RoutePattern()
					if routePattern != "" {
						if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
							labeler.Add(semconv.HTTPRoute(routePattern))
						}
						span := trace.SpanFromContext(r.Context())
						span.SetAttributes(semconv.HTTPRoute(routePattern))
						span.SetName(r.Method + " " + routePattern)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	})

	// Mount the handler on the mux
	genhttp.Mount(mux, genHttpServer)

	var handler http.Handler = mux

	// Add HTTP middleware
	// Note: Order matters - RequestIDMiddleware should come first in the chain,
	// so it should be the last middleware added to the handler since it is executed in reverse order.
	handler = middleware.RequestLoggerMiddleware()(handler)
	handler = middleware.RequestIDMiddleware()(handler)
	handler = middleware.AuthorizationMiddleware()(handler)
	// Cap total request body size to bound DoS exposure from unbounded multipart reads.
	// 1 MB headroom above the file limit covers multipart boundaries and all text fields.
	handler = middleware.BodyLimitMiddleware(models.MaxDocumentFileSize + 1<<20)(handler)
	// Wrap the handler with OpenTelemetry instrumentation
	handler = otelhttp.NewHandler(handler, "project-service",
		otelhttp.WithFilter(func(r *http.Request) bool {
			p := r.URL.Path
			return p != "/healthz" && p != "/livez" && p != "/readyz"
		}),
	)

	// Set up http listener in a goroutine using provided command line parameters.
	var addr string
	if flags.Bind == "*" {
		addr = ":" + flags.Port
	} else {
		addr = flags.Bind + ":" + flags.Port
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
	}
	gracefulCloseWG.Add(1)
	go func() {
		slog.With("addr", addr).Debug("starting http server, listening on port " + flags.Port)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			slog.With(errKey, err).Error("http listener error")
			os.Exit(1)
		}
		// Because ErrServerClosed is *immediately* returned when Shutdown is
		// called, not when when Shutdown completes, this must not yet decrement
		// the wait group.
	}()

	return httpServer
}

func setupNATS(ctx context.Context, env environment, svc *ProjectsAPI, gracefulCloseWG *sync.WaitGroup, done chan os.Signal) (*nats.Conn, error) {
	// Create NATS connection.
	gracefulCloseWG.Add(1)
	var err error
	slog.With("nats_url", env.NatsURL).Info("attempting to connect to NATS")
	natsConn, err := nats.Connect(
		env.NatsURL,
		nats.DrainTimeout(gracefulShutdownSeconds*time.Second),
		nats.ConnectHandler(func(_ *nats.Conn) {
			slog.With("nats_url", env.NatsURL).Info("NATS connection established")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
			if s != nil {
				slog.With(errKey, err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				slog.With(errKey, err).Error("async NATS error outside subscription")
			}
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			if ctx.Err() != nil {
				// If our parent background context has already been canceled, this is
				// a graceful shutdown. Decrement the wait group but do not exit, to
				// allow other graceful shutdown steps to complete.
				slog.With("nats_url", env.NatsURL).Info("NATS connection closed gracefully")
				gracefulCloseWG.Done()
				return
			}
			// Otherwise, this handler means that max reconnect attempts have been
			// exhausted.
			slog.With("nats_url", env.NatsURL).Error("NATS max-reconnects exhausted; connection closed")
			// Send a synthetic interrupt and give any graceful-shutdown tasks 5
			// seconds to clean up.
			done <- os.Interrupt
			time.Sleep(5 * time.Second)
			// Exit with an error instead of decrementing the wait group.
			os.Exit(1)
		}),
	)
	if err != nil {
		slog.With("nats_url", env.NatsURL, errKey, err).Error("error creating NATS client")
		return nil, err
	}

	// Get the key-value stores for the service.
	repo, err := getKeyValueStores(ctx, natsConn)
	if err != nil {
		return natsConn, err
	}
	svc.service.ProjectRepository = repo
	svc.service.DocumentRepository = repo
	svc.service.LinkRepository = repo
	svc.service.FolderRepository = repo

	svc.service.MessageBuilder = &internalnats.MessageBuilder{
		NatsConn: natsConn,
	}
	svc.service.UserReader = &internalnats.UserReaderNATS{
		NatsConn: natsConn,
	}

	// Create NATS subscriptions for the service.
	err = createNatsSubcriptions(ctx, svc, natsConn)
	if err != nil {
		return natsConn, err
	}

	return natsConn, nil
}

// getKeyValueStores creates a JetStream client and gets the key-value store for projects.
func getKeyValueStores(ctx context.Context, natsConn *nats.Conn) (*internalnats.NatsRepository, error) {
	kvStores := &internalnats.NatsRepository{}

	js, err := jetstream.New(natsConn)
	if err != nil {
		slog.ErrorContext(ctx, "error creating NATS JetStream client", "nats_url", natsConn.ConnectedUrl(), errKey, err)
		return kvStores, err
	}
	projectsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjects)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.KVStoreNameProjects)
		return kvStores, err
	}
	kvStores.Projects = projectsKV

	projectSettingsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectSettings)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.KVStoreNameProjectSettings)
		return kvStores, err
	}
	kvStores.ProjectSettings = projectSettingsKV

	linksKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectLinks)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.KVStoreNameProjectLinks)
		return kvStores, err
	}
	kvStores.Links = linksKV

	foldersKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectFolders)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.KVStoreNameProjectFolders)
		return kvStores, err
	}
	kvStores.Folders = foldersKV

	documentsKV, err := js.KeyValue(ctx, constants.KVStoreNameProjectDocuments)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream key-value store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.KVStoreNameProjectDocuments)
		return kvStores, err
	}
	kvStores.Documents = documentsKV

	documentFiles, err := js.ObjectStore(ctx, constants.ObjectStoreNameProjectDocuments)
	if err != nil {
		slog.ErrorContext(ctx, "error getting NATS JetStream object store", "nats_url", natsConn.ConnectedUrl(), errKey, err, "store", constants.ObjectStoreNameProjectDocuments)
		return kvStores, err
	}
	kvStores.DocumentFiles = documentFiles

	return kvStores, nil
}

// createNatsSubcriptions creates the NATS subscriptions for the project service.
func createNatsSubcriptions(ctx context.Context, svc *ProjectsAPI, natsConn *nats.Conn) error {
	slog.InfoContext(ctx, "subscribing to NATS subjects", "nats_url", natsConn.ConnectedUrl(), "servers", natsConn.Servers())
	queueName := constants.ProjectsAPIQueue

	for _, subject := range []string{
		// Get project name subscription
		constants.ProjectGetNameSubject,
		// Get project slug subscription
		constants.ProjectGetSlugSubject,
		// Get project logo subscription
		constants.ProjectGetLogoSubject,
		// Get project slug to UID subscription
		constants.ProjectSlugToUIDSubject,
		// Get project parent UID subscription
		constants.ProjectGetParentUIDSubject,
	} {
		slog.With("subject", subject, "queue", queueName).Debug("subscribing to NATS subject")
		_, err := natsConn.QueueSubscribe(subject, queueName, func(msg *nats.Msg) {
			msgCtx, end := internalnats.ExtractMsgContext(ctx, msg, subject)
			defer end()
			natsMsg := &internalnats.NatsMsg{Msg: msg}
			svc.service.HandleMessage(msgCtx, natsMsg)
		})
		if err != nil {
			slog.ErrorContext(ctx, "error creating NATS queue subscription", errKey, err)
			return err
		}
	}

	type eventHandler struct {
		subject string
		handle  func(ctx context.Context, msg domain.Message) error
	}
	for _, eh := range []eventHandler{
		{constants.ProjectSettingsUpdatedSubject, svc.service.HandleProjectSettingsUpdated},
		{inviteapi.InviteServiceAcceptedSubject, svc.service.HandleInviteAccepted},
		{constants.ProjectDocumentCreatedSubject, svc.service.HandleProjectDocumentCreated},
		{constants.ProjectLinkCreatedSubject, svc.service.HandleProjectLinkCreated},
	} {
		slog.With("subject", eh.subject, "queue", queueName).Debug("subscribing to NATS subject")
		_, err := natsConn.QueueSubscribe(eh.subject, queueName, func(msg *nats.Msg) {
			msgCtx, end := internalnats.ExtractMsgContext(ctx, msg, eh.subject)
			defer end()
			natsMsg := &internalnats.NatsMsg{Msg: msg}
			if handlerErr := eh.handle(msgCtx, natsMsg); handlerErr != nil {
				slog.WarnContext(msgCtx, "event handler failed", errKey, handlerErr, "subject", eh.subject)
			}
		})
		if err != nil {
			slog.ErrorContext(ctx, "error creating NATS queue subscription", errKey, err)
			return err
		}
	}

	return nil
}

func gracefulShutdown(httpServer *http.Server, natsConn *nats.Conn, gracefulCloseWG *sync.WaitGroup, cancel context.CancelFunc) {
	// Cancel the background context.
	cancel()

	go func() {
		// Run the HTTP shutdown in a goroutine so the NATS draining can also start.
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()

		slog.With("addr", httpServer.Addr).Info("shutting down http server")
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.With(errKey, err).Error("http shutdown error")
		}
		// Decrement the wait group.
		gracefulCloseWG.Done()
	}()

	// Drain the NATS connection, which will drain all subscriptions, then close the
	// connection when complete.
	if !natsConn.IsClosed() && !natsConn.IsDraining() {
		slog.Info("draining NATS connections")
		if err := natsConn.Drain(); err != nil {
			slog.With(errKey, err).Error("error draining NATS connection")
			// Skip waiting or checking error channel.
			return
		}
	}

	// Wait for the HTTP graceful shutdown and for the NATS connection to be
	// closed (see nats.Connect options for the timeout and the handler that
	// decrements the wait group).
	gracefulCloseWG.Wait()
}
