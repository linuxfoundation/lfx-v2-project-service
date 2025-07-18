// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRequestLoggerMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		userAgent      string
		remoteAddr     string
	}{
		{
			name:           "logs GET request correctly",
			method:         "GET",
			path:           "/projects",
			expectedStatus: http.StatusOK,
			userAgent:      "test-agent",
			remoteAddr:     "127.0.0.1:12345",
		},
		{
			name:           "logs POST request correctly",
			method:         "POST",
			path:           "/projects",
			expectedStatus: http.StatusCreated,
			userAgent:      "curl/7.68.0",
			remoteAddr:     "192.168.1.1:54321",
		},
		{
			name:           "logs error status correctly",
			method:         "GET",
			path:           "/nonexistent",
			expectedStatus: http.StatusNotFound,
			userAgent:      "Mozilla/5.0",
			remoteAddr:     "10.0.0.1:8080",
		},
	}

	assertion := assert.New(t)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test handler that returns the expected status
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Simulate some processing time
				time.Sleep(10 * time.Millisecond)
				w.WriteHeader(tc.expectedStatus)
				w.Write([]byte("test response"))
			})

			// Wrap with RequestLoggerMiddleware
			middleware := RequestLoggerMiddleware()
			wrappedHandler := middleware(handler)

			// Create request
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("User-Agent", tc.userAgent)
			req.RemoteAddr = tc.remoteAddr

			rec := httptest.NewRecorder()

			// Record start time
			start := time.Now()
			wrappedHandler.ServeHTTP(rec, req)
			duration := time.Since(start)

			// Verify response
			assertion.Equal(tc.expectedStatus, rec.Code)
			assertion.Equal("test response", rec.Body.String())

			// Verify duration is reasonable (should be at least 10ms due to sleep)
			assertion.GreaterOrEqual(duration, 10*time.Millisecond)
		})
	}
}

func TestResponseWriterWrapper(t *testing.T) {
	assertion := assert.New(t)

	// Create a mock response writer
	rec := httptest.NewRecorder()

	// Wrap it with our responseWriter
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// Test WriteHeader
	rw.WriteHeader(http.StatusNotFound)
	assertion.Equal(http.StatusNotFound, rw.statusCode)
	assertion.Equal(http.StatusNotFound, rec.Code)

	// Test Write
	content := []byte("test content")
	n, err := rw.Write(content)
	assertion.NoError(err)
	assertion.Equal(len(content), n)
	assertion.Equal(content, rec.Body.Bytes())
}

func BenchmarkRequestLoggerMiddleware(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	middleware := RequestLoggerMiddleware()
	wrappedHandler := middleware(handler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)
	}
}
