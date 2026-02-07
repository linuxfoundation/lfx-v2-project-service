// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package utils

import (
	"context"
	"testing"
)

// TestOTelConfigFromEnv verifies that OTelConfigFromEnv returns sensible
// defaults and correctly reads all supported OTEL_* environment variables.
func TestOTelConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := OTelConfigFromEnv()

		if cfg.ServiceName != "lfx-v2-project-service" {
			t.Errorf("expected default ServiceName 'lfx-v2-project-service', got %q", cfg.ServiceName)
		}
		if cfg.ServiceVersion != "" {
			t.Errorf("expected empty ServiceVersion, got %q", cfg.ServiceVersion)
		}
		if cfg.Protocol != OTelProtocolGRPC {
			t.Errorf("expected default Protocol %q, got %q", OTelProtocolGRPC, cfg.Protocol)
		}
		if cfg.Endpoint != "" {
			t.Errorf("expected empty Endpoint, got %q", cfg.Endpoint)
		}
		if cfg.Insecure != false {
			t.Errorf("expected Insecure false, got %t", cfg.Insecure)
		}
		if cfg.TracesExporter != OTelExporterNone {
			t.Errorf("expected default TracesExporter %q, got %q", OTelExporterNone, cfg.TracesExporter)
		}
		if cfg.TracesSampleRatio != 1.0 {
			t.Errorf("expected default TracesSampleRatio 1.0, got %f", cfg.TracesSampleRatio)
		}
		if cfg.MetricsExporter != OTelExporterNone {
			t.Errorf("expected default MetricsExporter %q, got %q", OTelExporterNone, cfg.MetricsExporter)
		}
		if cfg.LogsExporter != OTelExporterNone {
			t.Errorf("expected default LogsExporter %q, got %q", OTelExporterNone, cfg.LogsExporter)
		}
		if cfg.Propagators != "tracecontext,baggage" {
			t.Errorf("expected default Propagators 'tracecontext,baggage', got %q", cfg.Propagators)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "test-service")
		t.Setenv("OTEL_SERVICE_VERSION", "1.2.3")
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
		t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
		t.Setenv("OTEL_TRACES_SAMPLE_RATIO", "0.5")
		t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
		t.Setenv("OTEL_LOGS_EXPORTER", "otlp")
		t.Setenv("OTEL_PROPAGATORS", "tracecontext,baggage,jaeger")

		cfg := OTelConfigFromEnv()

		if cfg.ServiceName != "test-service" {
			t.Errorf("expected ServiceName 'test-service', got %q", cfg.ServiceName)
		}
		if cfg.ServiceVersion != "1.2.3" {
			t.Errorf("expected ServiceVersion '1.2.3', got %q", cfg.ServiceVersion)
		}
		if cfg.Protocol != OTelProtocolHTTP {
			t.Errorf("expected Protocol %q, got %q", OTelProtocolHTTP, cfg.Protocol)
		}
		if cfg.Endpoint != "localhost:4318" {
			t.Errorf("expected Endpoint 'localhost:4318', got %q", cfg.Endpoint)
		}
		if cfg.Insecure != true {
			t.Errorf("expected Insecure true, got %t", cfg.Insecure)
		}
		if cfg.TracesExporter != OTelExporterOTLP {
			t.Errorf("expected TracesExporter %q, got %q", OTelExporterOTLP, cfg.TracesExporter)
		}
		if cfg.TracesSampleRatio != 0.5 {
			t.Errorf("expected TracesSampleRatio 0.5, got %f", cfg.TracesSampleRatio)
		}
		if cfg.MetricsExporter != OTelExporterOTLP {
			t.Errorf("expected MetricsExporter %q, got %q", OTelExporterOTLP, cfg.MetricsExporter)
		}
		if cfg.LogsExporter != OTelExporterOTLP {
			t.Errorf("expected LogsExporter %q, got %q", OTelExporterOTLP, cfg.LogsExporter)
		}
		if cfg.Propagators != "tracecontext,baggage,jaeger" {
			t.Errorf("expected Propagators 'tracecontext,baggage,jaeger', got %q", cfg.Propagators)
		}
	})

	t.Run("unsupported protocol", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "unsupported")

		cfg := OTelConfigFromEnv()

		if cfg.Protocol != "unsupported" {
			t.Errorf("expected Protocol 'unsupported', got %q", cfg.Protocol)
		}
	})
}

// TestSetupOTelSDKWithConfig_AllDisabled verifies that the SDK can be
// initialized successfully when all exporters (traces, metrics, logs) are
// disabled, and that the returned shutdown function works correctly.
func TestSetupOTelSDKWithConfig_AllDisabled(t *testing.T) {
	cfg := OTelConfig{
		ServiceName:       "test-service",
		ServiceVersion:    "1.0.0",
		Protocol:          OTelProtocolGRPC,
		TracesExporter:    OTelExporterNone,
		TracesSampleRatio: 1.0,
		MetricsExporter:   OTelExporterNone,
		LogsExporter:      OTelExporterNone,
	}

	ctx := context.Background()
	shutdown, err := SetupOTelSDKWithConfig(ctx, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	// Call shutdown to ensure it works without error
	err = shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown returned unexpected error: %v", err)
	}
}

// TestNewResource verifies that newResource creates a valid OpenTelemetry
// resource with the expected service.name attribute for various input values.
func TestNewResource(t *testing.T) {
	tests := []struct {
		name           string
		serviceName    string
		serviceVersion string
	}{
		{"basic", "test-service", "1.0.0"},
		{"empty version", "test-service", ""},
		{"special chars", "test-service-123", "1.0.0-beta.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := OTelConfig{
				ServiceName:    tt.serviceName,
				ServiceVersion: tt.serviceVersion,
			}

			res, err := newResource(cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if res == nil {
				t.Fatal("expected non-nil resource")
			}

			// Verify resource contains expected attributes
			attrs := res.Attributes()
			found := false
			for _, attr := range attrs {
				if string(attr.Key) == "service.name" && attr.Value.AsString() == tt.serviceName {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("resource missing service.name attribute with value %q", tt.serviceName)
			}
		})
	}
}

// TestNewPropagator verifies that newPropagator returns a composite
// TextMapPropagator that includes the expected fields for each configuration.
func TestNewPropagator(t *testing.T) {
	tests := []struct {
		name           string
		propagators    string
		expectedFields []string
	}{
		{
			name:           "tracecontext and baggage",
			propagators:    "tracecontext,baggage",
			expectedFields: []string{"traceparent", "tracestate", "baggage"},
		},
		{
			name:           "tracecontext only",
			propagators:    "tracecontext",
			expectedFields: []string{"traceparent", "tracestate"},
		},
		{
			name:           "baggage only",
			propagators:    "baggage",
			expectedFields: []string{"baggage"},
		},
		{
			name:           "jaeger",
			propagators:    "jaeger",
			expectedFields: []string{"uber-trace-id"},
		},
		{
			name:           "all propagators",
			propagators:    "tracecontext,baggage,jaeger",
			expectedFields: []string{"traceparent", "tracestate", "baggage", "uber-trace-id"},
		},
		{
			name:           "unknown falls back to defaults",
			propagators:    "unknown",
			expectedFields: []string{"traceparent", "tracestate", "baggage"},
		},
		{
			name:           "with whitespace",
			propagators:    " tracecontext , baggage ",
			expectedFields: []string{"traceparent", "tracestate", "baggage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := OTelConfig{Propagators: tt.propagators}
			prop := newPropagator(cfg)

			if prop == nil {
				t.Fatal("expected non-nil propagator")
			}

			fields := prop.Fields()
			fieldSet := make(map[string]bool, len(fields))
			for _, f := range fields {
				fieldSet[f] = true
			}

			for _, expected := range tt.expectedFields {
				if !fieldSet[expected] {
					t.Errorf("expected propagator to include field %q, got fields %v", expected, fields)
				}
			}

			if len(fields) != len(tt.expectedFields) {
				t.Errorf("expected %d fields, got %d: %v", len(tt.expectedFields), len(fields), fields)
			}
		})
	}
}

// TestNormalizeEndpoint verifies that normalizeEndpoint correctly parses
// raw endpoint values into their full URL, scheme flag, and host components.
func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantFullURL   string
		wantHasScheme bool
		wantHost      string
	}{
		{
			name:          "host:port without scheme",
			raw:           "localhost:4317",
			wantFullURL:   "localhost:4317",
			wantHasScheme: false,
			wantHost:      "localhost:4317",
		},
		{
			name:          "http URL",
			raw:           "http://localhost:4318",
			wantFullURL:   "http://localhost:4318",
			wantHasScheme: true,
			wantHost:      "localhost:4318",
		},
		{
			name:          "https URL",
			raw:           "https://collector.example.com:4318",
			wantFullURL:   "https://collector.example.com:4318",
			wantHasScheme: true,
			wantHost:      "collector.example.com:4318",
		},
		{
			name:          "https URL with path",
			raw:           "https://collector.example.com:4318/v1/traces",
			wantFullURL:   "https://collector.example.com:4318/v1/traces",
			wantHasScheme: true,
			wantHost:      "collector.example.com:4318",
		},
		{
			name:          "hostname without port",
			raw:           "collector",
			wantFullURL:   "collector",
			wantHasScheme: false,
			wantHost:      "collector",
		},
		{
			name:          "http URL without port",
			raw:           "http://collector.example.com",
			wantFullURL:   "http://collector.example.com",
			wantHasScheme: true,
			wantHost:      "collector.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullURL, hasScheme, host := normalizeEndpoint(tt.raw)

			if fullURL != tt.wantFullURL {
				t.Errorf("fullURL = %q, want %q", fullURL, tt.wantFullURL)
			}
			if hasScheme != tt.wantHasScheme {
				t.Errorf("hasScheme = %t, want %t", hasScheme, tt.wantHasScheme)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
		})
	}
}

// TestOTelConstants verifies that the exported OTel constants have their
// expected string values, ensuring API compatibility.
func TestOTelConstants(t *testing.T) {
	if OTelProtocolGRPC != "grpc" {
		t.Errorf("expected OTelProtocolGRPC to be 'grpc', got %q", OTelProtocolGRPC)
	}
	if OTelProtocolHTTP != "http" {
		t.Errorf("expected OTelProtocolHTTP to be 'http', got %q", OTelProtocolHTTP)
	}
	if OTelExporterOTLP != "otlp" {
		t.Errorf("expected OTelExporterOTLP to be 'otlp', got %q", OTelExporterOTLP)
	}
	if OTelExporterNone != "none" {
		t.Errorf("expected OTelExporterNone to be 'none', got %q", OTelExporterNone)
	}
}
