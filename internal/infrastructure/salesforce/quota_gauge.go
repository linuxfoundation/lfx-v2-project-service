// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import "github.com/linuxfoundation/lfx-v2-member-service/internal/domain/port"

// APIUsageGauge implements port.SalesforceQuotaGauge by reading the
// SforceAPIUsageCurrent and SforceAPIUsageLimit expvars that are updated by
// rateLimitTransport on every Salesforce HTTP response.
type APIUsageGauge struct{}

// NewAPIUsageGauge creates an APIUsageGauge backed by the package-level
// SforceAPIUsageCurrent / SforceAPIUsageLimit expvars.
func NewAPIUsageGauge() *APIUsageGauge { return &APIUsageGauge{} }

// Ensure APIUsageGauge satisfies the port at compile time.
var _ port.SalesforceQuotaGauge = (*APIUsageGauge)(nil)

// APIUsage returns the most-recently observed Salesforce API usage counters.
// Returns (-1, -1) when no response has been received yet (both expvars are
// initialised to -1 and only updated after the first HTTP response).
func (g *APIUsageGauge) APIUsage() (current, limit int64) {
	return SforceAPIUsageCurrent.Value(), SforceAPIUsageLimit.Value()
}
