// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"log/slog"
	"strings"
)

// parseDomainAliases parses a raw Domain_Alias__c field value into a slice of
// valid, normalised domain strings. It handles the account-merge artifact
// format produced by Salesforce data-merge operations, which inserts CRLF
// sequences and a "--- Merged Data:" separator between formerly distinct alias
// lists (e.g. "pwc.com.pg\r\n\r\n--- Merged Data:\r\n\r\npwc.ai").
//
// Parsing rules:
//   - Split on commas AND newline characters (\r, \n).
//   - Trim leading/trailing whitespace from each token.
//   - Skip empty tokens.
//   - Skip tokens whose first non-whitespace characters are "---" (merge
//     separator artifacts).
//   - Apply normalizeDomain to each remaining token; append valid results.
//   - Warn on tokens that fail normalizeDomain.
//
// ctx and sfid are used only for structured warning log output.
func parseDomainAliases(ctx context.Context, sfid, raw string) []string {
	if raw == "" {
		return nil
	}

	// Replace newline variants with commas so a single Split covers all delimiters.
	cleaned := strings.NewReplacer("\r\n", ",", "\r", ",", "\n", ",").Replace(raw)

	tokens := strings.Split(cleaned, ",")
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// Skip Salesforce account-merge separator artifacts.
		if strings.HasPrefix(tok, "---") {
			continue
		}
		if normalized, ok := normalizeDomain(tok); ok {
			out = append(out, normalized)
		} else {
			slog.WarnContext(ctx, "account domain alias item does not look like a valid domain, omitting",
				"sfid", sfid,
				"raw_value", tok,
			)
		}
	}
	return out
}
