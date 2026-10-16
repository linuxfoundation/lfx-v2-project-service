// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

// RouteTaggingMiddleware creates a middleware that tags the OTel span with the
// matched chi route pattern.
//
// It must be registered inside chi's routing chain (via Mux.Use) so that
// http.route is set on the OTel span after chi has matched the route pattern.
// The span name is also updated here to avoid high-cardinality names from
// using raw URL paths (which contain actual path parameter values).
// Must be registered before Mount calls per chi convention.
// Reads RoutePattern after next.ServeHTTP because chi populates the pattern
// during routing (inside ServeHTTP), not before.
func RouteTaggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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
	}
}
