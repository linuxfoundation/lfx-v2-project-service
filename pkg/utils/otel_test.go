// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package utils

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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

	t.Run("invalid protocol falls back to grpc", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "unsupported")

		cfg := OTelConfigFromEnv()

		if cfg.Protocol != OTelProtocolGRPC {
			t.Errorf("expected Protocol %q, got %q", OTelProtocolGRPC, cfg.Protocol)
		}
	})

	t.Run("insecure accepts boolean variants", func(t *testing.T) {
		for _, v := range []string{"true", "TRUE", "True", "1", "t", "T"} {
			t.Run(v, func(t *testing.T) {
				t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", v)
				cfg := OTelConfigFromEnv()
				if !cfg.Insecure {
					t.Errorf("expected Insecure true for %q", v)
				}
			})
		}
		for _, v := range []string{"false", "FALSE", "0", "f", "F"} {
			t.Run(v, func(t *testing.T) {
				t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", v)
				cfg := OTelConfigFromEnv()
				if cfg.Insecure {
					t.Errorf("expected Insecure false for %q", v)
				}
			})
		}
	})

	t.Run("invalid insecure falls back to false", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "notabool")

		cfg := OTelConfigFromEnv()

		if cfg.Insecure {
			t.Errorf("expected Insecure false for invalid value, got true")
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

// TestSetupOTelSDKWithConfig_IPEndpoint verifies that SetupOTelSDKWithConfig
// normalizes a bare IP:port endpoint to include a scheme, preventing the
// "first path segment in URL cannot contain colon" error from the SDK.
func TestSetupOTelSDKWithConfig_IPEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4317")

	cfg := OTelConfig{
		ServiceName:       "test-service",
		ServiceVersion:    "1.0.0",
		Protocol:          OTelProtocolGRPC,
		Endpoint:          "127.0.0.1:4317",
		Insecure:          true,
		TracesExporter:    OTelExporterOTLP,
		TracesSampleRatio: 1.0,
		MetricsExporter:   OTelExporterNone,
		LogsExporter:      OTelExporterNone,
		Propagators:       "tracecontext,baggage",
	}

	ctx := context.Background()
	shutdown, err := SetupOTelSDKWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	_ = shutdown(ctx)
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

// TestEndpointURL verifies that endpointURL prepends the correct scheme
// when missing and preserves existing schemes.
func TestEndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		insecure bool
		want     string
	}{
		{
			name:     "IP:port insecure",
			raw:      "127.0.0.1:4317",
			insecure: true,
			want:     "http://127.0.0.1:4317",
		},
		{
			name:     "IP:port secure",
			raw:      "127.0.0.1:4317",
			insecure: false,
			want:     "https://127.0.0.1:4317",
		},
		{
			name:     "localhost:port insecure",
			raw:      "localhost:4317",
			insecure: true,
			want:     "http://localhost:4317",
		},
		{
			name:     "hostname without port",
			raw:      "collector",
			insecure: true,
			want:     "http://collector",
		},
		{
			name:     "http URL preserved",
			raw:      "http://collector.example.com:4318",
			insecure: false,
			want:     "http://collector.example.com:4318",
		},
		{
			name:     "https URL preserved",
			raw:      "https://collector.example.com:4318",
			insecure: true,
			want:     "https://collector.example.com:4318",
		},
		{
			name:     "https URL with path preserved",
			raw:      "https://collector.example.com:4318/v1/traces",
			insecure: false,
			want:     "https://collector.example.com:4318/v1/traces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointURL(tt.raw, tt.insecure)
			if got != tt.want {
				t.Errorf("endpointURL(%q, %t) = %q, want %q", tt.raw, tt.insecure, got, tt.want)
			}
		})
	}
}

// TestGRPCExporterEndpointFormats verifies which endpoint formats the OTel
// gRPC exporter accepts without error when passed via WithEndpoint or
// WithEndpointURL.
func TestGRPCExporterEndpointFormats(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    []otlptracegrpc.Option
		wantErr bool
	}{
		{
			name:    "WithEndpoint localhost:port",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpoint("localhost:4317"), otlptracegrpc.WithInsecure()},
			wantErr: false,
		},
		{
			name:    "WithEndpoint IP:port",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpoint("127.0.0.1:4317"), otlptracegrpc.WithInsecure()},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL http://localhost:port",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL("http://localhost:4317")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL http://IP:port",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL("http://127.0.0.1:4317")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL no scheme",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL("127.0.0.1:4317")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL https://IP:port",
			opts:    []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL("https://127.0.0.1:4317")},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := otlptracegrpc.New(ctx, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("otlptracegrpc.New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if exporter != nil {
				_ = exporter.Shutdown(ctx)
			}
		})
	}

	// Verify that the SDK reads OTEL_EXPORTER_OTLP_ENDPOINT env var
	// internally, which may call WithEndpointURL on bare IP:port values.
	t.Run("env var IP:port with WithEndpoint override", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4317")
		exporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint("127.0.0.1:4317"),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if exporter != nil {
			_ = exporter.Shutdown(ctx)
		}
	})

	t.Run("env var IP:port without explicit option", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4317")
		t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
		exporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if exporter != nil {
			_ = exporter.Shutdown(ctx)
		}
	})
}

// TestHTTPExporterEndpointFormats verifies which endpoint formats the OTel
// HTTP exporter accepts without error when passed via WithEndpoint or
// WithEndpointURL.
func TestHTTPExporterEndpointFormats(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    []otlptracehttp.Option
		wantErr bool
	}{
		{
			name:    "WithEndpoint localhost:port",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpoint("localhost:4318"), otlptracehttp.WithInsecure()},
			wantErr: false,
		},
		{
			name:    "WithEndpoint IP:port",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpoint("127.0.0.1:4318"), otlptracehttp.WithInsecure()},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL http://localhost:port",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpointURL("http://localhost:4318")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL http://IP:port",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpointURL("http://127.0.0.1:4318")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL no scheme",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpointURL("127.0.0.1:4318")},
			wantErr: false,
		},
		{
			name:    "WithEndpointURL https://IP:port",
			opts:    []otlptracehttp.Option{otlptracehttp.WithEndpointURL("https://127.0.0.1:4318")},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := otlptracehttp.New(ctx, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("otlptracehttp.New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if exporter != nil {
				_ = exporter.Shutdown(ctx)
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
