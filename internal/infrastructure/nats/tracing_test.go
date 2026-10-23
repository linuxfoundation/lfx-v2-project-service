// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"errors"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestNatsHeaderCarrier_Get(t *testing.T) {
	t.Run("returns empty string for missing key", func(t *testing.T) {
		carrier := natsHeaderCarrier(make(natsgo.Header))
		assert.Equal(t, "", carrier.Get("missing-key"))
	})

	t.Run("returns value for existing key", func(t *testing.T) {
		carrier := natsHeaderCarrier(natsgo.Header{
			"traceparent": []string{"00-trace-id-span-id-01"},
		})
		assert.Equal(t, "00-trace-id-span-id-01", carrier.Get("traceparent"))
	})

	t.Run("returns first value when multiple values set", func(t *testing.T) {
		carrier := natsHeaderCarrier(natsgo.Header{
			"key1": []string{"first", "second"},
		})
		assert.Equal(t, "first", carrier.Get("key1"))
	})

	t.Run("returns empty string for nil header", func(t *testing.T) {
		var carrier natsHeaderCarrier
		assert.Equal(t, "", carrier.Get("any-key"))
	})
}

func TestNatsHeaderCarrier_Set(t *testing.T) {
	t.Run("is a no-op for nil carrier", func(t *testing.T) {
		var carrier natsHeaderCarrier
		carrier.Set("key", "value") // must not panic
		assert.Equal(t, "", carrier.Get("key"))
	})

	t.Run("sets value on new key", func(t *testing.T) {
		carrier := natsHeaderCarrier(make(natsgo.Header))
		carrier.Set("traceparent", "00-abc-def-01")
		assert.Equal(t, "00-abc-def-01", carrier.Get("traceparent"))
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		carrier := natsHeaderCarrier(make(natsgo.Header))
		carrier.Set("key", "value1")
		carrier.Set("key", "value2")
		assert.Equal(t, "value2", carrier.Get("key"))
		assert.Equal(t, []string{"value2"}, natsgo.Header(carrier)["key"])
	})

	t.Run("stores full header state correctly", func(t *testing.T) {
		header := make(natsgo.Header)
		carrier := natsHeaderCarrier(header)
		carrier.Set("traceparent", "00-trace-span-01")
		carrier.Set("tracestate", "vendor=value")
		assert.Len(t, header, 2)
		assert.Equal(t, []string{"00-trace-span-01"}, header["traceparent"])
		assert.Equal(t, []string{"vendor=value"}, header["tracestate"])
	})
}

func TestNatsHeaderCarrier_Keys(t *testing.T) {
	t.Run("returns empty slice for empty header", func(t *testing.T) {
		carrier := natsHeaderCarrier(make(natsgo.Header))
		assert.Empty(t, carrier.Keys())
	})

	t.Run("returns all keys for populated header", func(t *testing.T) {
		carrier := natsHeaderCarrier(make(natsgo.Header))
		carrier.Set("key1", "v1")
		carrier.Set("key2", "v2")
		keys := carrier.Keys()
		assert.Len(t, keys, 2)
		assert.Contains(t, keys, "key1")
		assert.Contains(t, keys, "key2")
	})

	t.Run("returns empty slice for nil header", func(t *testing.T) {
		var carrier natsHeaderCarrier
		assert.Empty(t, carrier.Keys())
	})
}

func TestNatsHeaderCarrier_TextMapCarrier(t *testing.T) {
	t.Run("satisfies TextMapCarrier interface", func(t *testing.T) {
		var _ propagation.TextMapCarrier = natsHeaderCarrier{}
	})

	t.Run("inject and extract round-trip via propagator", func(t *testing.T) {
		header := make(natsgo.Header)
		carrier := natsHeaderCarrier(header)

		carrier.Set("traceparent", "00-trace-id-span-id-01")
		carrier.Set("tracestate", "vendor=value")

		assert.Equal(t, "00-trace-id-span-id-01", carrier.Get("traceparent"))
		assert.Equal(t, "vendor=value", carrier.Get("tracestate"))
		assert.Len(t, header, 2)
	})
}

func TestExtractMsgContext(t *testing.T) {
	t.Run("returns context and end function for nil header", func(t *testing.T) {
		msg := &natsgo.Msg{
			Subject: "test.subject",
			Data:    []byte("payload"),
		}
		ctx, end := ExtractMsgContext(context.Background(), msg, "test.subject")
		require.NotNil(t, ctx)
		require.NotNil(t, end)
		end() // must not panic
	})

	t.Run("returns context and end function for initialized header", func(t *testing.T) {
		msg := natsgo.NewMsg("test.subject")
		msg.Header = make(natsgo.Header)
		msg.Data = []byte("payload")

		ctx, end := ExtractMsgContext(context.Background(), msg, "test.subject")
		require.NotNil(t, ctx)
		require.NotNil(t, end)
		end() // must not panic
	})
}

func TestNewTracedMsg(t *testing.T) {
	setupAccessMessageTracing()

	t.Run("sets subject, data and an initialized header", func(t *testing.T) {
		msg := newTracedMsg(context.Background(), "test.subject", []byte("payload"))

		assert.Equal(t, "test.subject", msg.Subject)
		assert.Equal(t, []byte("payload"), msg.Data)
		assert.NotNil(t, msg.Header)
		assert.Empty(t, msg.Header.Get("traceparent"))
	})

	t.Run("injects trace context from a recording span", func(t *testing.T) {
		ctx, span := tracer.Start(context.Background(), "parent")
		defer span.End()

		msg := newTracedMsg(ctx, "test.subject", nil)

		assert.Contains(t, msg.Header.Get("traceparent"), span.SpanContext().TraceID().String())
	})
}

func TestRecordSpanError(t *testing.T) {
	spanRecorder := setupAccessMessageTracing()

	_, span := tracer.Start(context.Background(), "record-span-error-test")
	recordSpanError(span, errors.New("boom"))
	span.End()

	ended := spanRecorder.Ended()
	require.NotEmpty(t, ended)
	got := ended[len(ended)-1]
	assert.Equal(t, "record-span-error-test", got.Name())
	assert.Equal(t, codes.Error, got.Status().Code)
	assert.Equal(t, "boom", got.Status().Description)
	require.Len(t, got.Events(), 1)
	assert.Equal(t, "exception", got.Events()[0].Name)
}

func TestRequestWithSpan(t *testing.T) {
	spanRecorder := setupAccessMessageTracing()

	tests := []struct {
		name       string
		reqErr     error
		wantStatus codes.Code
	}{
		{name: "successful request returns the reply and leaves the status to the caller", wantStatus: codes.Unset},
		{name: "transport error is recorded on the span and returned unwrapped", reqErr: errors.New("timeout"), wantStatus: codes.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantReply := &natsgo.Msg{Data: []byte("reply")}
			var replyArg any
			if tt.reqErr == nil {
				replyArg = wantReply
			}
			conn := &MockNATSConn{}
			conn.On("RequestMsgWithContext", mock.Anything, mock.MatchedBy(func(msg *natsgo.Msg) bool {
				return msg.Subject == "test.request" &&
					string(msg.Data) == "payload" &&
					msg.Header.Get("traceparent") != ""
			})).Return(replyArg, tt.reqErr)

			ctx, span, reply, err := requestWithSpan(context.Background(), conn, "test.request", []byte("payload"))
			span.End()

			require.NotNil(t, ctx)
			if tt.reqErr != nil {
				assert.Equal(t, tt.reqErr, err)
				assert.Nil(t, reply)
			} else {
				require.NoError(t, err)
				assert.Equal(t, wantReply, reply)
			}
			conn.AssertExpectations(t)

			ended := spanRecorder.Ended()
			require.NotEmpty(t, ended)
			got := ended[len(ended)-1]
			assert.Equal(t, "nats.request", got.Name())
			assert.Equal(t, trace.SpanKindClient, got.SpanKind())
			assert.Equal(t, tt.wantStatus, got.Status().Code)
			assert.Contains(t, got.Attributes(), attribute.String("messaging.system", "nats"))
			assert.Contains(t, got.Attributes(), attribute.String("messaging.destination.name", "test.request"))
			assert.Contains(t, got.Attributes(), attribute.Int("messaging.message.body.size", len("payload")))
		})
	}
}
