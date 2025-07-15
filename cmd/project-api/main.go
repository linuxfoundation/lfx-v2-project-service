// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the project service API that provides a RESTful API for managing projects
// and handles NATS messages for the project service.
package main

import (
	"context"
	"embed"
	_ "expvar"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	goahttp "goa.design/goa/v3/http"

	genhttp "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/http/project_service/server"
	genquerysvc "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/log"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

//go:embed gen/http/openapi3.json gen/http/openapi3.yaml
var StaticFS embed.FS

const (
	errKey            = "error"
	defaultListenPort = "8080"
	// gracefulShutdownSeconds should be higher than NATS client
	// request timeout, and lower than the pod or liveness probe's
	// terminationGracePeriodSeconds.
	gracefulShutdownSeconds = 25
)

func main() {
	// Allow overriding the port by environmental variable as well as command
	// line argument.
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = defaultListenPort
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	lfxEnvironmentStr := os.Getenv("LFX_ENVIRONMENT")
	lfxEnvironment := constants.ParseLFXEnvironment(lfxEnvironmentStr)
	var debug = flag.Bool("d", false, "enable debug logging")
	var port = flag.String("p", defaultPort, "listen port")
	var bind = flag.String("bind", "*", "interface to bind on")

	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	// Set the log level environment variable used by [log.InitStructureLogConfig]
	if *debug {
		os.Setenv("LOG_LEVEL", "debug")
	}
	// Set up the logger
	log.InitStructureLogConfig()
	logger := slog.Default()

	// Set up JWT validator needed by the [ProjectsService.JWTAuth] security handler.
	jwtAuth := setupJWTAuth(logger)

	// Generated service initialization.
	svc := &ProjectsService{
		logger:         logger,
		lfxEnvironment: lfxEnvironment,
		auth:           jwtAuth,
	}

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

	handler := genhttp.New(
		endpoints,
		mux,
		requestDecoder,
		customEncoder,
		nil,
		nil,
		http.FS(StaticFS))

	// Mount the handler on the mux
	genhttp.Mount(mux, handler)

	// Create a wait group which is used to wait during graceful HTTP shutdown
	// and NATS draining.
	gracefulCloseWG := sync.WaitGroup{}

	// Set up http listener in a goroutine using provided command line parameters.
	var addr string
	if *bind == "*" {
		addr = ":" + *port
	} else {
		addr = *bind + ":" + *port
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	gracefulCloseWG.Add(1)
	go func() {
		logger.With("addr", addr).Debug("starting http server, listening on port " + *port)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.With(errKey, err).Error("http listener error")
			os.Exit(1)
		}
		// Because ErrServerClosed is *immediately* returned when Shutdown is
		// called, not when when Shutdown completes, this must not yet decrement
		// the wait group.
	}()

	// Support graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Create NATS connection.
	gracefulCloseWG.Add(1)
	var err error
	logger.With("nats_url", natsURL).Info("attempting to connect to NATS")
	natsConn, err := nats.Connect(
		natsURL,
		nats.DrainTimeout(gracefulShutdownSeconds*time.Second),
		nats.ConnectHandler(func(_ *nats.Conn) {
			logger.With("nats_url", natsURL).Info("NATS connection established")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
			if s != nil {
				logger.With(errKey, err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				logger.With(errKey, err).Error("async NATS error outside subscription")
			}
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			if ctx.Err() != nil {
				// If our parent background context has already been canceled, this is
				// a graceful shutdown. Decrement the wait group but do not exit, to
				// allow other graceful shutdown steps to complete.
				logger.With("nats_url", natsURL).Info("NATS connection closed gracefully")
				gracefulCloseWG.Done()
				return
			}
			// Otherwise, this handler means that max reconnect attempts have been
			// exhausted.
			logger.With("nats_url", natsURL).Error("NATS max-reconnects exhausted; connection closed")
			// Send a synthetic interrupt and give any graceful-shutdown tasks 5
			// seconds to clean up.
			done <- os.Interrupt
			time.Sleep(5 * time.Second)
			// Exit with an error instead of decrementing the wait group.
			os.Exit(1)
		}),
	)
	if err != nil {
		logger.With("nats_url", natsURL, errKey, err).Error("error creating NATS client")
		os.Exit(1)
	}
	svc.natsConn = natsConn

	// Get the key-value store for projects.
	svc.projectsKV, err = getKeyValueStore(ctx, svc, natsConn)
	if err != nil {
		os.Exit(1)
	}

	// Create NATS subscriptions for the project service.
	err = createNatsSubcriptions(svc, natsConn)
	if err != nil {
		os.Exit(1)
	}

	// This next line blocks until SIGINT or SIGTERM is received.
	<-done

	// Cancel the background context.
	cancel()

	go func() {
		// Run the HTTP shutdown in a goroutine so the NATS draining can also start.
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()

		logger.With("addr", httpServer.Addr).Info("shutting down http server")
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.With(errKey, err).Error("http shutdown error")
		}
		// Decrement the wait group.
		gracefulCloseWG.Done()
	}()

	// Drain the NATS connection, which will drain all subscriptions, then close the
	// connection when complete.
	if !natsConn.IsClosed() && !natsConn.IsDraining() {
		logger.Info("draining NATS connections")
		if err := natsConn.Drain(); err != nil {
			logger.With(errKey, err).Error("error draining NATS connection")
			// Skip waiting or checking error channel.
			os.Exit(1)
		}
	}

	// Wait for the HTTP graceful shutdown and for the NATS connection to be
	// closed (see nats.Connect options for the timeout and the handler that
	// decrements the wait group).
	gracefulCloseWG.Wait()
}

// getKeyValueStore creates a JetStream client and gets the key-value store for projects.
func getKeyValueStore(ctx context.Context, svc *ProjectsService, natsConn *nats.Conn) (jetstream.KeyValue, error) {
	js, err := jetstream.New(natsConn)
	if err != nil {
		svc.logger.With("nats_url", natsConn.ConnectedUrl(), errKey, err).Error("error creating NATS JetStream client")
		return nil, err
	}
	projectsKV, err := js.KeyValue(ctx, constants.KVBucketNameProjects)
	if err != nil {
		svc.logger.With("nats_url", natsConn.ConnectedUrl(), errKey, err, "bucket", constants.KVBucketNameProjects).Error("error getting NATS JetStream key-value store")
		return nil, err
	}
	return projectsKV, nil
}

// createNatsSubcriptions creates the NATS subscriptions for the project service.
func createNatsSubcriptions(svc *ProjectsService, natsConn *nats.Conn) error {
	svc.logger.
		With("nats_url", natsConn.ConnectedUrl()).
		With("servers", natsConn.Servers()).
		With("subjects", []string{constants.ProjectGetNameSubject, constants.ProjectSlugToUIDSubject}).
		Info("subscribing to NATS subjects")
	queueName := fmt.Sprintf("%s%s", svc.lfxEnvironment, constants.ProjectsAPIQueue)

	// Get project name subscription
	projectGetNameSubject := fmt.Sprintf("%s%s", svc.lfxEnvironment, constants.ProjectGetNameSubject)
	_, err := natsConn.QueueSubscribe(projectGetNameSubject, queueName, func(msg *nats.Msg) {
		svc.HandleNatsMessage(&NatsMsg{msg})
	})
	if err != nil {
		svc.logger.With(errKey, err).Error("error creating NATS queue subscription")
		return err
	}

	// Get project slug to UID subscription
	projectSlugToUIDSubject := fmt.Sprintf("%s%s", svc.lfxEnvironment, constants.ProjectSlugToUIDSubject)
	_, err = natsConn.QueueSubscribe(projectSlugToUIDSubject, queueName, func(msg *nats.Msg) {
		svc.HandleNatsMessage(&NatsMsg{msg})
	})
	if err != nil {
		svc.logger.With(errKey, err).Error("error creating NATS queue subscription")
		return err
	}

	return nil
}
