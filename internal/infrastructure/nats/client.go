// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/env"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const defaultNATSURL = "nats://localhost:4222"

// NatsConfig holds NATS connection settings.
type NatsConfig struct {
	URL           string
	Timeout       time.Duration
	MaxReconnect  int
	ReconnectWait time.Duration
}

// NatsConfigFromEnv builds NatsConfig using NATS_URL when set.
func NatsConfigFromEnv() NatsConfig {
	url := env.Get("NATS_URL", defaultNATSURL)
	return NatsConfig{
		URL:           url,
		Timeout:       10 * time.Second,
		MaxReconnect:  3,
		ReconnectWait: 2 * time.Second,
	}
}

// Connect establishes a NATS connection and JetStream context.
func Connect(_ context.Context, cfg NatsConfig) (*nats.Conn, jetstream.JetStream, error) {
	if cfg.URL == "" {
		cfg.URL = defaultNATSURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 3
	}
	if cfg.ReconnectWait == 0 {
		cfg.ReconnectWait = 2 * time.Second
	}

	nc, err := nats.Connect(cfg.URL,
		nats.Timeout(cfg.Timeout),
		nats.MaxReconnects(cfg.MaxReconnect),
		nats.ReconnectWait(cfg.ReconnectWait),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return nc, js, nil
}
