// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"

	"github.com/nats-io/nats.go"
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
// so trace context can be injected into NATS message headers.
type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string {
	vals := c[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c natsHeaderCarrier) Set(key string, value string) {
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

// ExtractMsgContext extracts OTel trace context from an incoming NATS message
// header and starts a Consumer span. Call the returned function (typically via
// defer) to end the span.
func ExtractMsgContext(ctx context.Context, msg *nats.Msg, subject string) (context.Context, func()) {
	var hdr nats.Header
	if msg.Header != nil {
		hdr = msg.Header
	}
	msgCtx := otel.GetTextMapPropagator().Extract(ctx, natsHeaderCarrier(hdr))
	msgCtx, span := tracer.Start(msgCtx, "nats.process",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.operation.type", "process"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(msg.Data)),
		),
	)
	return msgCtx, func() {
		span.End()
	}
}
