// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package env

import "testing"

func TestGet(t *testing.T) {
	t.Setenv("TEST_ENV_KEY", "from-env")
	if got := Get("TEST_ENV_KEY", "default"); got != "from-env" {
		t.Fatalf("Get = %q, want from-env", got)
	}

	t.Setenv("TEST_ENV_TRIM", "  trimmed  ")
	if got := Get("TEST_ENV_TRIM", "default"); got != "trimmed" {
		t.Fatalf("Get trimmed = %q, want trimmed", got)
	}

	if got := Get("TEST_ENV_UNSET", "default"); got != "default" {
		t.Fatalf("Get unset = %q, want default", got)
	}
}

func TestGetBool(t *testing.T) {
	cases := []struct {
		env      string
		def      bool
		expected bool
	}{
		{"true", false, true},
		{"FALSE", true, false},
		{"yes", false, true},
		{"0", true, false},
		{"", true, true},
		{"maybe", true, true},
	}
	for _, c := range cases {
		if c.env != "" {
			t.Setenv("TEST_BOOL", c.env)
		} else {
			t.Setenv("TEST_BOOL", "")
		}
		if got := GetBool("TEST_BOOL", c.def); got != c.expected {
			t.Fatalf("GetBool(%q, %v) = %v, want %v", c.env, c.def, got, c.expected)
		}
	}
}

func TestGetInt(t *testing.T) {
	t.Setenv("TEST_INT", "25")
	if got := GetInt("TEST_INT", 50); got != 25 {
		t.Fatalf("GetInt = %d, want 25", got)
	}

	t.Setenv("TEST_INT", "0")
	if got := GetInt("TEST_INT", 50); got != 0 {
		t.Fatalf("GetInt zero = %d, want 0", got)
	}

	if got := GetInt("TEST_INT_UNSET", 50); got != 50 {
		t.Fatalf("GetInt unset = %d, want 50", got)
	}
}
