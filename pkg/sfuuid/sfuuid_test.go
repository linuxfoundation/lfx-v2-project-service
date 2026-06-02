// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sfuuid

import (
	"testing"
)

// TestNormalize15 verifies Normalize15 across valid and invalid inputs.
func TestNormalize15(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// 15-char inputs — pass through unchanged.
		{"15_account", "001B000000IqhSL", "001B000000IqhSL", false},
		{"15_contact", "003B0000001ckSl", "003B0000001ckSl", false},
		{"15_project", "a0941000002wBz9", "a0941000002wBz9", false},
		{"15_all_low", "AAAAAAAAAAAAAAA", "AAAAAAAAAAAAAAA", false},
		{"15_all_high", "999999999999999", "999999999999999", false},

		// 18-char inputs — suffix stripped, first 15 chars returned.
		{"18_account", "001B000000IqhSLIAZ", "001B000000IqhSL", false},
		{"18_contact", "003B0000001ckSlIAI", "003B0000001ckSl", false},
		{"18_project", "a0941000002wBz9AAE", "a0941000002wBz9", false},

		// Stability: both 15- and 18-char forms of the same SFID produce the same result.
		{"stability_15", "001B000000IqhSL", "001B000000IqhSL", false},
		{"stability_18", "001B000000IqhSLIAZ", "001B000000IqhSL", false},

		// Error cases.
		{"too_short", "ABC123", "", true},
		{"too_long_not_18", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "", true},
		{"empty", "", "", true},
		{"invalid_char", "AAAAAAAAAAAAA!@", "", true},
		// Note: for 18-char inputs only the first 15 chars are validated (suffix is stripped).
		// "001B000000IqhSL!@#" has a valid 15-char prefix so it is accepted — this is correct.
		{"invalid_char_in_prefix", "AAAAAAAAAAAAA!@IAAZ", "", true}, // invalid char in first 15
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize15(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Normalize15(%q) expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize15(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("Normalize15(%q) = %q, want %q", tc.input, got, tc.want)
			}
			// Result must always be exactly 15 chars.
			if len(got) != 15 {
				t.Errorf("Normalize15(%q) returned %d chars, want 15", tc.input, len(got))
			}
		})
	}
}

// TestNormalize15RoundTripWithSalesforce15To18 verifies that
// Normalize15(Salesforce15To18(id15)) == id15 for valid 15-char inputs.
func TestNormalize15RoundTripWithSalesforce15To18(t *testing.T) {
	cases := []string{
		"001B000000IqhSL",
		"003B0000001ckSl",
		"a0941000002wBz9",
		"AAAAAAAAAAAAAAA",
		"999999999999999",
	}

	for _, id15 := range cases {
		t.Run(id15, func(t *testing.T) {
			id18, err := Salesforce15To18(id15)
			if err != nil {
				t.Fatalf("Salesforce15To18(%q) error: %v", id15, err)
			}
			if len(id18) != 18 {
				t.Fatalf("Salesforce15To18(%q) = %q: expected 18 chars", id15, id18)
			}

			got, err := Normalize15(id18)
			if err != nil {
				t.Fatalf("Normalize15(%q) error: %v", id18, err)
			}
			if got != id15 {
				t.Errorf("Normalize15(Salesforce15To18(%q)) = %q, want %q", id15, got, id15)
			}
		})
	}
}

// TestIsSFID verifies the IsSFID predicate.
func TestIsSFID(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"15_valid", "001B000000IqhSL", true},
		{"18_valid", "001B000000IqhSLIAZ", true},
		{"too_short", "ABC123", false},
		{"empty", "", false},
		{"invalid_char", "AAAAAAAAAAAAA!@", false},
		{"uuid_not_sfid", "4c46585f-9f01-8bda-a0a5-f0c8eeef7fff", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSFID(tc.input)
			if got != tc.want {
				t.Errorf("IsSFID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestSalesforce15To18 verifies the 15→18 expansion.
func TestSalesforce15To18(t *testing.T) {
	cases := []struct {
		id15  string
		want  string
		error bool
	}{
		{"001B000000IqhSL", "001B000000IqhSLIAZ", false},
		{"003B0000001ckSl", "003B0000001ckSlIAI", false},
		{"a0941000002wBz9", "a0941000002wBz9AAE", false},
		{"", "", true},
		{"tooshort", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.id15, func(t *testing.T) {
			got, err := Salesforce15To18(tc.id15)
			if tc.error {
				if err == nil {
					t.Errorf("Salesforce15To18(%q) expected error, got nil", tc.id15)
				}
				return
			}
			if err != nil {
				t.Fatalf("Salesforce15To18(%q) unexpected error: %v", tc.id15, err)
			}
			if got != tc.want {
				t.Errorf("Salesforce15To18(%q) = %q, want %q", tc.id15, got, tc.want)
			}
		})
	}
}

func TestNormalize18(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "15-char input expanded to 18-char",
			input: "001B000000IqhSL",
			want:  "001B000000IqhSLIAZ",
		},
		{
			name:  "18-char input returned unchanged",
			input: "001B000000IqhSLIAZ",
			want:  "001B000000IqhSLIAZ",
		},
		{
			name:  "another 15-char SFID",
			input: "a0941000002wBz9",
			want:  "a0941000002wBz9AAE",
		},
		{
			name:    "empty string is an error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "UUID format rejected",
			input:   "00000000-0000-0000-0000-000000000001",
			wantErr: true,
		},
		{
			name:    "17-char string rejected",
			input:   "001B000000IqhSLIA",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize18(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Normalize18(%q) expected error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize18(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("Normalize18(%q) = %q, want %q", tc.input, got, tc.want)
			}
			if len(got) != 18 {
				t.Errorf("Normalize18(%q) returned %d chars, want 18", tc.input, len(got))
			}
		})
	}
}
