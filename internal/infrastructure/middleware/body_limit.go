// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package middleware

import "net/http"

// BodyLimitMiddleware wraps every incoming request body with http.MaxBytesReader
// so that no request can stream more than limit bytes regardless of how many
// multipart parts or fields it contains. Once the limit is breached,
// subsequent reads return an error and the connection is closed.
func BodyLimitMiddleware(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
