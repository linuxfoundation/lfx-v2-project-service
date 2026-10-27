// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// tracer is safe to initialize at package level — otel.Tracer() returns a
// delegating tracer that forwards to whatever TracerProvider is registered at
// call time, so otel.SetTracerProvider() updates it regardless of init order.
var tracer = otel.Tracer("github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats")

// natsHeaderCarrier adapts nats.Header to the OTel TextMapCarrier interface
// so trace context can be injected/extracted from NATS message headers.
// Callers must provide an initialized (non-nil) header map before injection;
// Set is a no-op on a nil map to safely support extraction from messages
// that arrive without headers.
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

// newTracedMsg builds a NATS message with the OTel trace context injected into its headers.
func newTracedMsg(ctx context.Context, subject string, data []byte) *natsgo.Msg {
	msg := natsgo.NewMsg(subject)
	msg.Header = make(natsgo.Header)
	msg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(msg.Header))
	return msg
}

// requestWithSpan sends a core-NATS request under an OTel client span and returns the
// span's context, the span and the reply. The caller owns the span: it must defer
// span.End() before checking err, and record the outcome of anything it does with the
// reply via recordSpanError or span.SetStatus. A transport error is recorded on the span
// before it is returned unwrapped.
func requestWithSpan(ctx context.Context, conn INatsConn, subject string, data []byte) (context.Context, trace.Span, *natsgo.Msg, error) {
	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)

	reply, err := conn.RequestMsgWithContext(ctx, newTracedMsg(ctx, subject, data))
	if err != nil {
		recordSpanError(span, err)
		return ctx, span, nil, err
	}
	return ctx, span, reply, nil
}

// requestAllWithSpan is requestWithSpan for lookups whose reply drives an identity or
// authorization decision: it returns every reply received within window of the first one,
// so the caller can reject a spoofed answer that disagrees with the real responder's.
// The span contract is the same as requestWithSpan.
func requestAllWithSpan(ctx context.Context, conn INatsConn, subject string, data []byte, window time.Duration) (context.Context, trace.Span, []*natsgo.Msg, error) {
	ctx, span := tracer.Start(ctx, "nats.request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.Int("messaging.message.body.size", len(data)),
		),
	)

	replies, err := CollectReplies(ctx, NewReplyConn(conn), newTracedMsg(ctx, subject, data), window)
	if err != nil {
		recordSpanError(span, err)
		return ctx, span, nil, err
	}
	span.SetAttributes(attribute.Int("messaging.reply.count", len(replies)))
	return ctx, span, replies, nil
}

// recordSpanError records err on the span and marks the span as failed.
func recordSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
