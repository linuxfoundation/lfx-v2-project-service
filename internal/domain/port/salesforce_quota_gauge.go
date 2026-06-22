// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package port

// SalesforceQuotaGauge reports the most-recently observed Salesforce REST API
// usage for the current 24-hour rolling window. The gauge is updated by
// rateLimitTransport on every HTTP response; values of -1 indicate the signal
// has not yet been observed (e.g. no response received yet).
type SalesforceQuotaGauge interface {
	// APIUsage returns the most-recently observed (current, limit) API call
	// counts. Returns (-1, -1) when no response has been received yet.
	APIUsage() (current, limit int64)
}
