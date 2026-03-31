// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const (
	sforceLimitInfoHeader = "Sforce-Limit-Info"
	apiUsagePrefix        = "api-usage="
)

// SforceAPIUsageCurrent is the most-recently observed Salesforce API call count
// for the current 24-hour rolling window, parsed from the Sforce-Limit-Info
// response header (e.g. "api-usage=150/15000"). Initialised to -1 to indicate
// "not yet observed" — distinct from a legitimate zero usage count.
var SforceAPIUsageCurrent = func() *expvar.Int {
	v := expvar.NewInt("sfdc_api_usage_current")
	v.Set(-1)
	return v
}()

// SforceAPIUsageLimit is the maximum Salesforce API calls allowed in the
// current 24-hour rolling window, parsed from the Sforce-Limit-Info response
// header (e.g. "api-usage=150/15000"). Initialised to -1 to indicate
// "not yet observed".
var SforceAPIUsageLimit = func() *expvar.Int {
	v := expvar.NewInt("sfdc_api_usage_limit")
	v.Set(-1)
	return v
}()

// rateLimitTransport is an http.RoundTripper that wraps an inner transport,
// inspects every Salesforce response for the Sforce-Limit-Info header, and
// keeps the SforceAPIUsageCurrent / SforceAPIUsageLimit expvar counters
// up-to-date. All requests are delegated transparently to the inner transport.
type rateLimitTransport struct {
	inner http.RoundTripper
}

// NewRateLimitTransport returns an http.RoundTripper that wraps inner and
// updates the Salesforce API-usage expvar counters on every response.
// If inner is nil, http.DefaultTransport is used.
func NewRateLimitTransport(inner http.RoundTripper) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &rateLimitTransport{inner: inner}
}

// RoundTrip delegates to the inner transport and then parses the
// Sforce-Limit-Info header from the response, if present.
func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	if header := resp.Header.Get(sforceLimitInfoHeader); header != "" {
		current, limit, parseErr := parseAPIUsage(header)
		if parseErr != nil {
			slog.Warn("failed to parse Sforce-Limit-Info header",
				"error", parseErr,
				"header", header,
			)
		} else {
			SforceAPIUsageCurrent.Set(current)
			SforceAPIUsageLimit.Set(limit)
		}
	}

	return resp, nil
}

// parseAPIUsage extracts the current and limit integers from a
// Sforce-Limit-Info header value. The expected format contains a segment of
// the form "api-usage=<current>/<limit>", e.g.:
//
//	"api-usage=150/15000"
//	"api-usage=150/15000; per-app-api-usage=17/250(appName=MyConnectedApp)"
//
// Returns an error if the segment is missing or malformed.
func parseAPIUsage(header string) (current, limit int64, err error) {
	// The header may contain multiple semicolon-separated segments.
	for segment := range strings.SplitSeq(header, ";") {
		segment = strings.TrimSpace(segment)
		if !strings.HasPrefix(segment, apiUsagePrefix) {
			continue
		}

		value := strings.TrimPrefix(segment, apiUsagePrefix)
		before, after, ok := strings.Cut(value, "/")
		if !ok {
			return 0, 0, fmt.Errorf("salesforce: malformed api-usage segment %q", segment)
		}

		current, err = strconv.ParseInt(strings.TrimSpace(before), 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("salesforce: parsing api-usage current %q: %w", before, err)
		}

		limit, err = strconv.ParseInt(strings.TrimSpace(after), 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("salesforce: parsing api-usage limit %q: %w", after, err)
		}

		return current, limit, nil
	}

	return 0, 0, fmt.Errorf("salesforce: %q segment not found in Sforce-Limit-Info header %q", apiUsagePrefix, header)
}
