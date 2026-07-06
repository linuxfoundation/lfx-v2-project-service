// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package opensearch provides a shared OpenSearch client for operational tooling.
package opensearch

import (
	"context"
	"fmt"
	"os"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"
)

const defaultURL = "http://localhost:9200"

// Config holds OpenSearch connection settings.
type Config struct {
	URL string
}

// ConfigFromEnv builds Config using OPENSEARCH_URL when set.
func ConfigFromEnv() Config {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		url = defaultURL
	}
	return Config{URL: url}
}

// NewClient creates an OpenSearch client from the given configuration.
func NewClient(_ context.Context, cfg Config) (*opensearchgo.Client, error) {
	if cfg.URL == "" {
		cfg.URL = defaultURL
	}
	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses: []string{cfg.URL},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}
	return client, nil
}
