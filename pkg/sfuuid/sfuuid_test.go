// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sfuuid

import (
	"strings"
	"testing"
)

// TestRoundTrip18 verifies that 18-char Salesforce IDs survive a ToUUID →
// ToSFID round-trip, with the result matching the first 15 characters of the
// input (the canonical form).
func TestRoundTrip18(t *testing.T) {
	cases := []struct {
		name string
		sfid string
	}{
		{"account", "001B000000IqhSLIAZ"},
		{"contact", "003B0000001ckSlIAI"},
		{"project", "a0941000002wBz9AAE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want15 := tc.sfid[:15]

			uuidStr, err := ToUUID(tc.sfid)
			if err != nil {
				t.Fatalf("ToUUID(%q) error: %v", tc.sfid, err)
			}

			// UUID must be in standard 8-4-4-4-12 hyphenated form.
			if len(uuidStr) != 36 || strings.Count(uuidStr, "-") != 4 {
				t.Fatalf("ToUUID(%q) = %q: not a standard UUID string", tc.sfid, uuidStr)
			}

			// Version nibble must be 8.
			if uuidStr[14] != '8' {
				t.Errorf("ToUUID(%q) version nibble = %c, want 8", tc.sfid, uuidStr[14])
			}

			// Variant bits must be 8, 9, a, or b (RFC 9562 / RFC 4122 variant).
			varNibble := uuidStr[19]
			if varNibble != '8' && varNibble != '9' && varNibble != 'a' && varNibble != 'b' {
				t.Errorf("ToUUID(%q) variant nibble = %c, want 8/9/a/b", tc.sfid, varNibble)
			}

			got, err := ToSFID(uuidStr)
			if err != nil {
				t.Fatalf("ToSFID(%q) error: %v", uuidStr, err)
			}
			if got != want15 {
				t.Errorf("round-trip(%q): got %q, want %q", tc.sfid, got, want15)
			}

			// Confirm the 18-char expansion of the recovered 15-char ID is valid.
			got18, err := Salesforce15To18(got)
			if err != nil {
				t.Fatalf("Salesforce15To18(%q) error: %v", got, err)
			}
			if got18[:15] != want15 {
				t.Errorf("Salesforce15To18(%q)[:15] = %q, want %q", got, got18[:15], want15)
			}

			t.Logf("%-18s → %s → %s (18-char: %s)", tc.sfid, uuidStr, got, got18)
		})
	}
}

// TestRoundTrip15 verifies that 15-char Salesforce IDs survive a ToUUID →
// ToSFID round-trip unchanged.
func TestRoundTrip15(t *testing.T) {
	cases := []struct {
		name string
		sfid string
	}{
		{"account_15", "001B000000IqhSL"},
		{"contact_15", "003B0000001ckSl"},
		{"project_15", "a0941000002wBz9"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uuidStr, err := ToUUID(tc.sfid)
			if err != nil {
				t.Fatalf("ToUUID(%q) error: %v", tc.sfid, err)
			}

			got, err := ToSFID(uuidStr)
			if err != nil {
				t.Fatalf("ToSFID(%q) error: %v", uuidStr, err)
			}
			if got != tc.sfid {
				t.Errorf("round-trip(%q): got %q, want %q", tc.sfid, got, tc.sfid)
			}

			t.Logf("%-15s → %s → %s", tc.sfid, uuidStr, got)
		})
	}
}

// TestRoundTripEdgeCases verifies the low and high ends of the base-62 alphabet.
func TestRoundTripEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		sfid string
	}{
		{"all_low", "AAAAAAAAAAAAAAA"},  // Lowest character in b62Chars (index 0).
		{"all_high", "999999999999999"}, // Highest character in b62Chars (index 61).
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uuidStr, err := ToUUID(tc.sfid)
			if err != nil {
				t.Fatalf("ToUUID(%q) error: %v", tc.sfid, err)
			}

			if len(uuidStr) != 36 || strings.Count(uuidStr, "-") != 4 {
				t.Fatalf("ToUUID(%q) = %q: not a standard UUID string", tc.sfid, uuidStr)
			}

			got, err := ToSFID(uuidStr)
			if err != nil {
				t.Fatalf("ToSFID(%q) error: %v", uuidStr, err)
			}
			if got != tc.sfid {
				t.Errorf("round-trip(%q): got %q, want %q", tc.sfid, got, tc.sfid)
			}

			t.Logf("%-15s → %s → %s", tc.sfid, uuidStr, got)
		})
	}
}

// TestSFID18And15Equivalence verifies that the 15-char and 18-char forms of
// the same Salesforce ID produce identical UUIDs.
func TestSFID18And15Equivalence(t *testing.T) {
	cases := []struct {
		sfid18 string
	}{
		{"001B000000IqhSLIAZ"},
		{"003B0000001ckSlIAI"},
		{"a0941000002wBz9AAE"},
	}

	for _, tc := range cases {
		t.Run(tc.sfid18, func(t *testing.T) {
			sfid15 := tc.sfid18[:15]

			uuid18, err := ToUUID(tc.sfid18)
			if err != nil {
				t.Fatalf("ToUUID(%q) error: %v", tc.sfid18, err)
			}

			uuid15, err := ToUUID(sfid15)
			if err != nil {
				t.Fatalf("ToUUID(%q) error: %v", sfid15, err)
			}

			if uuid18 != uuid15 {
				t.Errorf("ToUUID(%q) = %q, ToUUID(%q) = %q: expected identical UUIDs",
					tc.sfid18, uuid18, sfid15, uuid15)
			}
		})
	}
}

// TestToSFIDRejectsNonLFX verifies that ToSFID returns an error for UUIDs that
// are not LFX_ UUID v8 values (wrong version, wrong variant, wrong namespace).
func TestToSFIDRejectsNonLFX(t *testing.T) {
	cases := []struct {
		name    string
		uuidStr string
	}{
		{"uuid_v4", "550e8400-e29b-41d4-a716-446655440000"},
		{"uuid_v5", "2ed6657d-e927-568b-95e3-af9fed2da63c"},
		{"all_zeros", "00000000-0000-0000-0000-000000000000"},
		{"wrong_ns", "deadbeef-cafe-8abc-8012-000000000000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ToSFID(tc.uuidStr)
			if err == nil {
				t.Errorf("ToSFID(%q) expected error, got nil", tc.uuidStr)
			}
		})
	}
}

// TestToUUIDRejectsInvalidInput verifies that ToUUID returns an error when
// given input that is not a valid Salesforce ID.
func TestToUUIDRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"too_short", "ABC123"},
		{"too_long", "ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
		{"invalid_chars", "AAAAAAAAAAAAA!@"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ToUUID(tc.input)
			if err == nil {
				t.Errorf("ToUUID(%q) expected error, got nil", tc.input)
			}
		})
	}
}
