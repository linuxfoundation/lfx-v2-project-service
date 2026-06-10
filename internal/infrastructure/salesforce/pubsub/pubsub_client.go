// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package pubsub implements the CDCSubscriber port using Salesforce Pub/Sub
// gRPC API v1. It owns all gRPC, proto, and Avro types; nothing from those
// layers crosses into the domain.
package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/model"
	"github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"
	pbproto "github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/salesforce/pubsub/proto"
)

const (
	// defaultBatchSize is the number of events requested per FetchRequest.
	// Salesforce recommends 100 as a reasonable upper bound.
	defaultBatchSize = 100

	// reconnectDelay is the backoff between Subscribe stream reconnects.
	reconnectDelay = 5 * time.Second
)

// Ensure Client satisfies port.CDCSubscriber at compile time.
var _ port.CDCSubscriber = (*Client)(nil)

// tokenProvider is a function that returns the current Salesforce access token
// and instance URL. Called once per gRPC stream to refresh credentials.
type tokenProvider func() (accessToken, instanceURL, orgID string)

// Client is the CDCSubscriber adapter. It wraps the Salesforce Pub/Sub gRPC
// service, handles Avro schema caching, and emits normalized CDCEvents.
// Implements port.CDCSubscriber.
type Client struct {
	grpcConn    *grpc.ClientConn
	stub        pbproto.PubSubClient
	tokenFn     tokenProvider
	schemaCache sync.Map // map[schemaID string]string       — raw Avro JSON from GetSchema
	codecCache  sync.Map // map[schemaID string]*goavro.Codec — compiled codec, reused across events
	batchSize   int32
}

// NewClient dials the Salesforce Pub/Sub gRPC endpoint and returns a Client.
// The tokenFn is called each time a new stream is opened so that a refreshed
// access token is always used (Salesforce sessions expire).
//
// endpoint example: "api.pubsub.salesforce.com:7443"
func NewClient(endpoint string, tokenFn tokenProvider) (*Client, error) {
	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("pubsub: dial %s: %w", endpoint, err)
	}

	return &Client{
		grpcConn:  conn,
		stub:      pbproto.NewPubSubClient(conn),
		tokenFn:   tokenFn,
		batchSize: defaultBatchSize,
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	return c.grpcConn.Close()
}

// Subscribe implements port.CDCSubscriber. It opens a bidirectional gRPC
// stream, requests events in batches, decodes Avro payloads, and emits
// CDCEvents on the returned channel.
//
// On transient errors (network blip, stream EOF) it reconnects automatically
// with a short delay. Before each reconnect attempt it reloads the cursor from
// replayStore.Load — this is the same store the consumer commits to after each
// event. Reloading on reconnect ensures that if an event was delivered to the
// channel but the consumer had not yet committed it before the disconnect, it
// is re-delivered rather than silently skipped.
//
// replayID nil means start at LATEST (skip historical events).
func (c *Client) Subscribe(ctx context.Context, channel string, replayID []byte, replayStore port.ReplayStore) (<-chan model.CDCEvent, error) {
	out := make(chan model.CDCEvent, defaultBatchSize)

	go func() {
		defer close(out)
		first := true
		for {
			// On first attempt use the caller-supplied cursor (which was loaded
			// from the store by CDCConsumer.Run before calling Subscribe).
			// On reconnects reload from the store so we resume from the last
			// committed position, not from the last in-flight delivery.
			var (
				current []byte
				err     error
			)
			if first {
				current = replayID
				first = false
			} else {
				current, err = replayStore.Load(ctx, channel)
				if err != nil {
					slog.ErrorContext(ctx, "pubsub: failed to reload replay cursor, stopping",
						"channel", channel, "error", err)
					return
				}
			}

			streamErr := c.runStream(ctx, channel, current, out)
			if streamErr == nil || ctx.Err() != nil {
				return
			}
			slog.WarnContext(ctx, "pubsub: stream ended, reconnecting",
				"channel", channel,
				"error", streamErr,
				"delay", reconnectDelay,
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectDelay):
			}
		}
	}()

	return out, nil
}

// runStream opens one Subscribe stream and forwards events to out.
// Returns nil only when ctx is cancelled; returns an error for reconnectable failures.
// Subscribe reloads the cursor from the persistent store before each reconnect,
// so there is no need to track the in-flight cursor here.
func (c *Client) runStream(
	ctx context.Context,
	channel string,
	replayID []byte,
	out chan<- model.CDCEvent,
) error {
	accessToken, instanceURL, orgID := c.tokenFn()

	// Inject Salesforce auth metadata required by the gRPC server.
	md := metadata.Pairs(
		"accesstoken", accessToken,
		"instanceurl", instanceURL,
		"tenantid", orgID,
	)
	streamCtx := metadata.NewOutgoingContext(ctx, md)

	stream, err := c.stub.Subscribe(streamCtx)
	if err != nil {
		return fmt.Errorf("pubsub: open stream: %w", err)
	}

	// Send initial FetchRequest.
	req := &pbproto.FetchRequest{
		TopicName:    channel,
		NumRequested: c.batchSize,
	}
	if len(replayID) > 0 {
		req.ReplayPreset = pbproto.ReplayPreset_CUSTOM
		req.ReplayId = replayID
	} else {
		req.ReplayPreset = pbproto.ReplayPreset_LATEST
	}

	if err := stream.Send(req); err != nil {
		return fmt.Errorf("pubsub: send fetch request: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("pubsub: recv: %w", err)
		}

		for _, event := range resp.GetEvents() {
			cdcEvent, decErr := c.decodeEvent(ctx, event)
			if decErr != nil {
				slog.ErrorContext(ctx, "pubsub: decode event, skipping",
					"schema_id", event.GetEvent().GetSchemaId(),
					"error", decErr,
				)
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			case out <- cdcEvent:
			}
		}

		// Request next batch to keep the stream flowing.
		if err := stream.Send(&pbproto.FetchRequest{
			TopicName:    channel,
			NumRequested: c.batchSize,
		}); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("pubsub: send next fetch: %w", err)
		}
	}
}

// fetchSchema retrieves and caches the Avro schema for the given schema ID.
// Cached per the Salesforce recommendation: schemas change rarely; only re-fetch
// when a new schema_id appears in a FetchResponse.
func (c *Client) fetchSchema(ctx context.Context, schemaID string) (string, error) {
	if v, ok := c.schemaCache.Load(schemaID); ok {
		return v.(string), nil
	}

	accessToken, instanceURL, orgID := c.tokenFn()
	md := metadata.Pairs(
		"accesstoken", accessToken,
		"instanceurl", instanceURL,
		"tenantid", orgID,
	)
	schemaCtx := metadata.NewOutgoingContext(ctx, md)

	resp, err := c.stub.GetSchema(schemaCtx, &pbproto.SchemaRequest{SchemaId: schemaID})
	if err != nil {
		return "", fmt.Errorf("pubsub: GetSchema %s: %w", schemaID, err)
	}

	schemaJSON := resp.GetSchemaJson()
	c.schemaCache.Store(schemaID, schemaJSON)

	slog.DebugContext(ctx, "pubsub: schema cached", "schema_id", schemaID)
	return schemaJSON, nil
}
