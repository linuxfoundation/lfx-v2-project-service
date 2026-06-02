// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package sfuuid provides Salesforce ID utilities for the LFX member service.
//
// The canonical uid for every Salesforce-backed entity is the 18-char SFID
// (15-char base-62 Salesforce ID + 3-char case-encoding suffix). Using 18-char
// ensures interoperability with other services that receive IDs directly from
// Salesforce, which returns 18-char IDs by default. IsSFID validates, Normalize18
// converts any 15- or 18-char input to canonical 18-char form, and Normalize15
// strips the suffix to the 15-char form when needed for Salesforce API path params.
//
// Only the project uid is a real v2 UUID (sourced from project-service over
// NATS); all other entity uids are 18-char SFIDs.
package sfuuid

import (
	"fmt"
	"strings"
)

// b62Chars is the Salesforce base-62 alphabet. Order must match across all implementations.
const b62Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// suffixChars maps a 5-bit value (0-31) to its suffix character for 18-char IDs.
const suffixChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"

// b62Index maps ASCII byte → base-62 index (-1 = invalid character).
var b62Index [256]int8

func init() {
	for i := range b62Index {
		b62Index[i] = -1
	}
	for i, ch := range b62Chars {
		b62Index[byte(ch)] = int8(i)
	}
}

// IsSFID reports whether s is a syntactically valid 15- or 18-character
// Salesforce ID (all characters in the base-62 alphabet, with the 18-char form
// including a valid 3-character case-encoding suffix).
func IsSFID(s string) bool {
	_, err := normalize(s)
	return err == nil
}

// Normalize18 returns the canonical 18-char Salesforce ID for a 15- or 18-char
// input. A 15-char ID has its 3-char case-encoding suffix appended; an 18-char
// ID is validated and returned as-is. All characters are validated against the
// Salesforce base-62 alphabet. An error is returned for any other input.
//
// This is the single normalisation step for Salesforce-backed entity uids: it
// produces a stable 18-char identifier suitable as a cache key, FGA object id,
// or OpenSearch doc _id, regardless of whether the caller received a 15- or
// 18-char SFID from Salesforce.
func Normalize18(s string) (string, error) {
	id15, err := normalize(s)
	if err != nil {
		return "", err
	}
	return Salesforce15To18(id15)
}

// Normalize15 returns the canonical 15-char Salesforce ID for a 15- or 18-char
// input. The 3-char case-encoding suffix of an 18-char ID is stripped; all
// characters are validated against the Salesforce base-62 alphabet. An error is
// returned for any other input (wrong length, invalid characters).
func Normalize15(s string) (string, error) {
	return normalize(s)
}

// Salesforce15To18 appends the standard 3-character case-encoding suffix to a
// 15-char Salesforce ID, producing the portable 18-char case-insensitive form.
//
// The suffix encodes which positions in the base ID hold uppercase letters.
// Characters are grouped into three sets of five; each group yields one suffix
// character via a 5-bit mask (bit j set ↔ position j is uppercase A-Z), mapped
// through suffixChars ("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345").
func Salesforce15To18(id15 string) (string, error) {
	if len(id15) != 15 {
		return "", fmt.Errorf("uid: expected 15-char Salesforce ID, got %d chars", len(id15))
	}
	var sb strings.Builder
	sb.Grow(18)
	sb.WriteString(id15)
	for group := 0; group < 3; group++ {
		var bits int
		for j := 0; j < 5; j++ {
			ch := id15[group*5+j]
			if ch >= 'A' && ch <= 'Z' {
				bits |= 1 << j
			}
		}
		sb.WriteByte(suffixChars[bits])
	}
	return sb.String(), nil
}

// normalize returns the canonical 15-char form, stripping the 3-char suffix if
// an 18-char ID is provided, and validating all base-62 characters.
func normalize(sfid string) (string, error) {
	if len(sfid) == 18 {
		sfid = sfid[:15]
	}
	if len(sfid) != 15 {
		return "", fmt.Errorf("uid: expected 15- or 18-char Salesforce ID, got %d chars", len(sfid))
	}
	for i := 0; i < len(sfid); i++ {
		if b62Index[sfid[i]] < 0 {
			return "", fmt.Errorf("uid: invalid character %q at position %d", sfid[i], i)
		}
	}
	return sfid, nil
}
