// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	goahttp "goa.design/goa/v3/http"

	genhttp "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/http/project_service/server"
	genquerysvc "github.com/linuxfoundation/lfx-v2-project-service/api/project/v1/gen/project_service"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/middleware"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// newHTTPHandler builds the full HTTP handler for the service: the Goa
// endpoints mounted on a muxer, wrapped in the request middleware chain and
// OpenTelemetry instrumentation.
func newHTTPHandler(svc *ProjectsAPI) http.Handler {
	// Wrap it in the generated endpoints
	endpoints := genquerysvc.NewEndpoints(svc)

	// Build an HTTP handler
	mux := goahttp.NewMuxer()
	requestDecoder := projectRequestDecoder
	responseEncoder := goahttp.ResponseEncoder

	koDataPath := os.Getenv("KO_DATA_PATH")
	if koDataPath == "" {
		koDataPath = "./api/project/v1/gen/http/"
	}

	koDataDir := http.Dir(koDataPath)

	genHttpServer := genhttp.New(
		endpoints,
		mux,
		requestDecoder,
		responseEncoder,
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
	// Must be registered before Mount calls per chi convention.
	mux.Use(middleware.RouteTaggingMiddleware())

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

	return handler
}

// listenAddr returns the address the HTTP server listens on. A bind value of
// "*" listens on all interfaces.
func listenAddr(flags cmdFlags) string {
	if flags.Bind == "*" {
		return ":" + flags.Port
	}
	return flags.Bind + ":" + flags.Port
}

func setupHTTPServer(flags cmdFlags, svc *ProjectsAPI, gracefulCloseWG *sync.WaitGroup) *http.Server {
	handler := newHTTPHandler(svc)

	// Set up http listener in a goroutine using provided command line parameters.
	addr := listenAddr(flags)
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
			slog.With(constants.ErrKey, err).Error("http listener error")
			os.Exit(1)
		}
		// Because ErrServerClosed is *immediately* returned when Shutdown is
		// called, not when when Shutdown completes, this must not yet decrement
		// the wait group.
	}()

	return httpServer
}
