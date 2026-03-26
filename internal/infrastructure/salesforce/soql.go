// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import "strings"

// quoteSOQL escapes a string value for safe interpolation into a SOQL query.
// Per the Salesforce SOQL reserved-character rules, both the backslash (\) and
// single-quote (') characters must be escaped with a leading backslash. The
// backslash is escaped first so that the escape sequences introduced for single
// quotes are not themselves re-escaped. The result is wrapped in single quotes,
// producing a value suitable for use in a SOQL WHERE clause (e.g. WHERE Id =
// 'abc123').
//
// SOQL does not support parameterized queries, so all external values must be
// escaped before interpolation. Use this function for every user-supplied or
// externally-sourced string that is substituted into a SOQL query string.
func quoteSOQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", `\'`)
	return "'" + s + "'"
}

// escapeLikeSOQL escapes a user-supplied search term for safe embedding inside
// a SOQL LIKE string literal. It escapes backslashes and single quotes (as
// quoteSOQL does) and additionally escapes the SOQL wildcard characters % and _
// so they are treated as literals rather than pattern metacharacters.
//
// The returned string is unquoted. Use it together with quoteSOQL to build a
// contains-style pattern:
//
//	quoteSOQL("%" + escapeLikeSOQL(term) + "%")
//
// This ensures the surrounding % wildcards are preserved as pattern
// metacharacters while any % or _ in the user input are treated as literals.
func escapeLikeSOQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", `\'`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// buildSOQLInClause builds a comma-separated list of quoted, escaped values
// suitable for a SOQL IN clause (e.g. 'a','b','c'). Each value is passed
// through quoteSOQL so that embedded single quotes are safely escaped.
//
// Returns "”" (an empty quoted string) when values is empty, which produces a
// syntactically valid but always-false IN predicate.
func buildSOQLInClause(values []string) string {
	if len(values) == 0 {
		return "''"
	}

	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = quoteSOQL(v)
	}

	return strings.Join(parts, ",")
}
