// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"

	natsgo "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// tracer is safe to initialize at package level — otel.Tracer() returns a
// delegating tracer that forwards to whatever TracerProvider is registered at
// call time, so otel.SetTracerProvider() updates it regardless of init order.
var tracer = otel.Tracer("github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats")

// natsHeaderCarrier adapts nats.Header to the OTel TextMapCarrier interface
// so trace context can be injected/extracted from NATS message headers.
type natsHeaderCarrier natsgo.Header

func (c natsHeaderCarrier) Get(key string) string {
	vals := c[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c natsHeaderCarrier) Set(key string, value string) {
	if c == nil {
		return
	}
	c[key] = []string{value}
}

func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

var _ propagation.TextMapCarrier = natsHeaderCarrier{}

// ExtractTraceContext extracts the OTel trace context from NATS message headers.
func ExtractTraceContext(ctx context.Context, header natsgo.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, natsHeaderCarrier(header))
}

// ExtractMsgContext extracts trace context from NATS message headers and starts a consumer span.
// It returns a new context with the extracted trace and a function to end the span.
// The returned function must be called with defer to ensure the span is properly closed.
func ExtractMsgContext(ctx context.Context, msg *natsgo.Msg, subject string) (context.Context, func()) {
	msgCtx := ExtractTraceContext(ctx, msg.Header)
	msgCtx, span := tracer.Start(msgCtx, "nats.process",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.operation.type", "process"),
			attribute.Int("messaging.message.body.size", len(msg.Data)),
		),
	)
	return msgCtx, func() { span.End() }
}
