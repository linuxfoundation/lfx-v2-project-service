// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package nats

import "time"

// Config represents NATS configuration
type Config struct {
	// URL is the NATS server URL
	URL string `json:"url"`
	// Timeout is the request timeout duration
	Timeout time.Duration `json:"timeout"`
	// MaxReconnect is the maximum number of reconnection attempts
	MaxReconnect int `json:"max_reconnect"`
	// ReconnectWait is the time to wait between reconnection attempts
	ReconnectWait time.Duration `json:"reconnect_wait"`
	// OnClosed is called when the NATS connection is permanently closed after
	// all reconnect attempts are exhausted. If nil, the closure is only logged.
	// Callers should use this to signal the process to exit so Kubernetes can
	// restart the pod rather than leaving it running with a dead connection.
	OnClosed func()
}
