// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRouteTaggingMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		wantSpanName string
		wantRoute    string
		wantStatus   int
	}{
		{
			name:         "span is named after the route pattern, not the raw path",
			method:       http.MethodGet,
			path:         "/projects/00000000-0000-0000-0000-000000000001",
			wantSpanName: "GET /projects/{uid}",
			wantRoute:    "/projects/{uid}",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "static routes keep their path",
			method:       http.MethodGet,
			path:         "/projects",
			wantSpanName: "GET /projects",
			wantRoute:    "/projects",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "unmatched routes leave the span untouched",
			method:       http.MethodGet,
			path:         "/does-not-exist",
			wantSpanName: "original",
			wantStatus:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

			router := chi.NewRouter()
			router.Use(RouteTaggingMiddleware())
			router.Get("/projects", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			router.Get("/projects/{uid}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

			ctx, span := tp.Tracer("test").Start(t.Context(), "original")
			req := httptest.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			span.End()

			assert.Equal(t, tt.wantStatus, res.Code)

			ended := recorder.Ended()
			require.Len(t, ended, 1)
			assert.Equal(t, tt.wantSpanName, ended[0].Name())

			var gotRoute string
			for _, kv := range ended[0].Attributes() {
				if kv.Key == attribute.Key("http.route") {
					gotRoute = kv.Value.AsString()
				}
			}
			assert.Equal(t, tt.wantRoute, gotRoute)
		})
	}

	t.Run("the wrapped handler is always called", func(t *testing.T) {
		called := false
		handler := RouteTaggingMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
		}))

		// No chi route context and no active span: must not panic.
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

		assert.True(t, called)
	})
}
