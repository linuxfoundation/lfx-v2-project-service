// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/linuxfoundation/lfx-v2-member-service/pkg/constants"
	"github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSClient wraps the NATS connection and provides KV store access.
type NATSClient struct {
	conn    *nats.Conn
	config  Config
	kvStore map[string]jetstream.KeyValue
	timeout time.Duration
}

// Conn returns the underlying *nats.Conn for callers that need direct
// NATS request/reply access (e.g., the project resolver).
func (c *NATSClient) Conn() *nats.Conn {
	return c.conn
}

// Close gracefully closes the NATS connection.
func (c *NATSClient) Close() error {
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

// IsReady checks if the NATS client is ready.
func (c *NATSClient) IsReady(ctx context.Context) error {
	if c.conn == nil {
		return errors.NewServiceUnavailable("NATS client is not initialized or not connected")
	}
	if !c.conn.IsConnected() || c.conn.IsDraining() {
		return errors.NewServiceUnavailable("NATS client is not ready, connection is not established or is draining")
	}
	return nil
}

// KeyValueStore creates a JetStream client and gets the key-value store for the
// given bucket. For the membership-cache bucket, a 24-hour MaxAge TTL is set
// when creating a new bucket. If the bucket already exists, the existing
// configuration is used as-is — NATS does not allow in-place TTL updates on
// live buckets; the Helm chart is responsible for pre-creating it with the
// correct config.
func (c *NATSClient) KeyValueStore(ctx context.Context, bucketName string) error {
	js, err := jetstream.New(c.conn)
	if err != nil {
		slog.ErrorContext(ctx, "error creating NATS JetStream client",
			"error", err,
			"nats_url", c.conn.ConnectedUrl(),
		)
		return err
	}
	kvStore, err := js.KeyValue(ctx, bucketName)
	if err != nil {
		slog.InfoContext(ctx, "KV bucket not found, creating it",
			"bucket", bucketName,
		)
		cfg := jetstream.KeyValueConfig{
			Bucket: bucketName,
		}
		// Both cache buckets use a 24-hour hard-eviction TTL. For membership-cache
		// the soft-TTL logic lives in the CachedValue envelope; for
		// member-service-cache freshness is governed by HTTP conditional GET
		// semantics (ETag / Last-Modified) and the bucket TTL acts as a backstop.
		if bucketName == constants.KVBucketNameCache || bucketName == constants.KVBucketNameSObjectCache {
			cfg.TTL = 24 * time.Hour
		}
		kvStore, err = js.CreateKeyValue(ctx, cfg)
		if err != nil {
			slog.ErrorContext(ctx, "error creating NATS JetStream key-value store",
				"error", err,
				"nats_url", c.conn.ConnectedUrl(),
				"bucket", bucketName,
			)
			return err
		}
	}

	if c.kvStore == nil {
		c.kvStore = make(map[string]jetstream.KeyValue)
	}
	c.kvStore[bucketName] = kvStore
	return nil
}

// NewClient creates a new NATS client with the given configuration.
func NewClient(ctx context.Context, config Config) (*NATSClient, error) {
	slog.InfoContext(ctx, "creating NATS client",
		"url", config.URL,
		"timeout", config.Timeout,
	)

	if config.URL == "" {
		return nil, errors.NewUnexpected("NATS URL is required")
	}

	opts := []nats.Option{
		nats.Name(constants.ServiceName),
		nats.Timeout(config.Timeout),
		nats.MaxReconnects(config.MaxReconnect),
		nats.ReconnectWait(config.ReconnectWait),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			slog.WarnContext(ctx, "NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.InfoContext(ctx, "NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
			if s != nil {
				slog.With("error", err, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				slog.With("error", err).Error("async NATS error outside subscription")
			}
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			// If the context is already done, this is an intentional shutdown;
			// do nothing.
			if ctx.Err() != nil {
				slog.InfoContext(ctx, "NATS connection closed during shutdown")
				return
			}
			slog.Error("NATS max-reconnects exhausted; connection permanently closed")
			if config.OnClosed != nil {
				config.OnClosed()
			} else {
				// Default behaviour: give the logger a moment to flush, then exit
				// so Kubernetes restarts the pod rather than leaving it running
				// with a dead NATS connection.
				time.Sleep(5 * time.Second)
				os.Exit(1)
			}
		}),
	}

	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, errors.NewServiceUnavailable("failed to connect to NATS", err)
	}

	client := &NATSClient{
		conn:    conn,
		config:  config,
		timeout: config.Timeout,
	}

	if err := client.KeyValueStore(ctx, constants.KVBucketNameCache); err != nil {
		slog.ErrorContext(ctx, "failed to initialize NATS key-value store",
			"error", err,
			"bucket", constants.KVBucketNameCache,
		)
		return nil, errors.NewServiceUnavailable("failed to initialize NATS key-value store", err)
	}
	slog.InfoContext(ctx, "NATS key-value store initialized",
		"bucket", constants.KVBucketNameCache,
	)

	if err := client.KeyValueStore(ctx, constants.KVBucketNameSObjectCache); err != nil {
		slog.ErrorContext(ctx, "failed to initialize NATS sObject key-value store",
			"error", err,
			"bucket", constants.KVBucketNameSObjectCache,
		)
		return nil, errors.NewServiceUnavailable("failed to initialize NATS sObject key-value store", err)
	}
	slog.InfoContext(ctx, "NATS key-value store initialized",
		"bucket", constants.KVBucketNameSObjectCache,
	)

	slog.InfoContext(ctx, "NATS client created successfully",
		"connected_url", conn.ConnectedUrl(),
		"status", conn.Status(),
	)

	return client, nil
}
