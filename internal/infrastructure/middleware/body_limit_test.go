// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		limit      int64
		body       string
		wantStatus int
	}{
		{
			name:       "body within limit passes through",
			limit:      10,
			body:       "hello",
			wantStatus: http.StatusOK,
		},
		{
			name:       "body exactly at limit passes through",
			limit:      5,
			body:       "hello",
			wantStatus: http.StatusOK,
		},
		{
			name:       "body exceeding limit causes read error",
			limit:      3,
			body:       "hello",
			wantStatus: http.StatusOK, // handler still runs, but Read fails mid-stream
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, readErr = io.ReadAll(r.Body)
			})

			handler := BodyLimitMiddleware(tt.limit)(inner)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.limit < int64(len(tt.body)) {
				if readErr == nil {
					t.Error("expected read error for oversized body, got nil")
				}
			} else {
				if readErr != nil {
					t.Errorf("unexpected read error for within-limit body: %v", readErr)
				}
			}
		})
	}
}
