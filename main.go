// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
// The project service.
package main

import (
	"context"
	"embed"
	_ "expvar"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	goahttp "goa.design/goa/v3/http"

	genhttp "github.com/linuxfoundation/lfx-v2-project-service/gen/http/project_service/server"
	genquerysvc "github.com/linuxfoundation/lfx-v2-project-service/gen/project_service"
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

var (
	logger         *slog.Logger
	natsURL        string
	resourcesIndex string
	natsConn       *nats.Conn
	client         *opensearchapi.Client
)

func init() {
	natsURL = os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}
}

func main() {
	// Allow overriding the port by environmental variable as well as command
	// line argument.
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = defaultListenPort
	}
	var debug = flag.Bool("d", false, "enable debug logging")
	var port = flag.String("p", defaultPort, "listen port")
	var bind = flag.String("bind", "*", "interface to bind on")

	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	logOptions := &slog.HandlerOptions{}

	// Optional debug logging.
	if os.Getenv("DEBUG") != "" || *debug {
		logOptions.Level = slog.LevelDebug
		logOptions.AddSource = true
	}

	logger = slog.New(slog.NewJSONHandler(os.Stdout, logOptions))
	slog.SetDefault(logger)

	// Set up JWT validator needed by the JWTAuth security handler.
	setupJWTAuth()

	// Generated service initialization.
	svc := &ProjectsService{}

	// Wrap it in the generated endpoints
	endpoints := genquerysvc.NewEndpoints(svc)

	// Build an HTTP handler
	mux := goahttp.NewMuxer()
	requestDecoder := goahttp.RequestDecoder
	responseEncoder := goahttp.ResponseEncoder
	handler := genhttp.New(
		endpoints,
		mux,
		requestDecoder,
		responseEncoder,
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
		slog.With("addr", addr).Info("starting http server, listening on port " + *port)
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
	//ctx, cancel := context.WithCancel(context.Background())
	//defer cancel()
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Create NATS connection.
	//gracefulCloseWG.Add(1)
	//var err error
	//natsConn, err = nats.Connect(
	//	natsURL,
	//	nats.DrainTimeout(gracefulShutdownSeconds*time.Second),
	//	nats.ErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
	//		if s != nil {
	//			logger.With(errKey, err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
	//		} else {
	//			logger.With(errKey, err).Error("async NATS error outside subscription")
	//		}
	//	}),
	//	nats.ClosedHandler(func(_ *nats.Conn) {
	//		if ctx.Err() != nil {
	//			// If our parent background context has already been canceled, this is
	//			// a graceful shutdown. Decrement the wait group but do not exit, to
	//			// allow other graceful shutdown steps to complete.
	//			gracefulCloseWG.Done()
	//			return
	//		}
	//		// Otherwise, this handler means that max reconnect attempts have been
	//		// exhausted.
	//		logger.Error("NATS max-reconnects exhausted; connection closed")
	//		// Send a synthetic interrupt and give any graceful-shutdown tasks 5
	//		// seconds to clean up.
	//		done <- os.Interrupt
	//		time.Sleep(5 * time.Second)
	//		// Exit with an error instead of decrementing the wait group.
	//		os.Exit(1)
	//	}),
	//)
	// if err != nil {
	// 	logger.With(errKey, err).Error("error creating NATS client")
	// 	os.Exit(1)
	// }

	// This next line blocks until SIGINT or SIGTERM is received.
	<-done

	// Cancel the background context.
	//cancel()

	go func() {
		// Run the HTTP shutdown in a goroutine so the NATS draining can also start.
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownSeconds*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			logger.With(errKey, err).Error("http shutdown error")
		}
		// Decrement the wait group.
		gracefulCloseWG.Done()
	}()

	// Drain the connection, which will drain all subscriptions, then close the
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
