// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package salesforce provides shared helper utilities for Salesforce SOQL query
// construction and result parsing.
package salesforce

import (
	"time"
)

// parseSOQLTime parses a Salesforce datetime string into a time.Time value.
// Salesforce returns dates in ISO 8601 format (e.g.
// "2024-01-15T10:30:45.123+0000"). Returns the zero value on parse failure.
//
// Sub-second precision is preserved: the ".000" in the reference format string
// is a Go layout placeholder for milliseconds, so ".123" parses to 123ms.
func parseSOQLTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	// Salesforce datetime format with milliseconds and UTC offset.
	// Go's time.Parse treats the ".000" in the layout as a fractional-seconds
	// placeholder, preserving whatever sub-second value is present in s.
	t, err := time.Parse("2006-01-02T15:04:05.000+0000", s)
	if err != nil {
		// Try RFC3339Nano as a fallback (also preserves sub-second precision).
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			// Try RFC3339 next.
			t, err = time.Parse(time.RFC3339, s)
			if err != nil {
				// Try date-only format as a last resort.
				t, err = time.Parse("2006-01-02", s)
				if err != nil {
					return time.Time{}
				}
			}
		}
	}

	return t
}

// parseSOQLDateTime parses a Salesforce datetime pointer into a formatted
// RFC3339 string. Returns an empty string on parse failure or nil input.
func parseSOQLDateTime(s *string) string {
	if s == nil || *s == "" {
		return ""
	}

	t := parseSOQLTime(*s)
	if t.IsZero() {
		return ""
	}

	return t.Format(time.RFC3339)
}
