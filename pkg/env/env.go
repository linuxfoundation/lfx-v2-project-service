// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package env provides helpers for reading environment variables with defaults.
package env

import (
	"fmt"
	"os"
	"strings"
)

// Get returns the trimmed value of the environment variable named by key.
// If the variable is unset or empty after trimming, defaultValue is returned.
func Get(key, defaultValue string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultValue
}

// GetBool returns the boolean value of the environment variable named by key.
// Recognizes true/false, 1/0, t/f, yes/no (case-insensitive).
// If the variable is unset or unrecognized, defaultValue is returned.
func GetBool(key string, defaultValue bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	switch strings.ToLower(v) {
	case "true", "1", "t", "yes":
		return true
	case "false", "0", "f", "no":
		return false
	default:
		return defaultValue
	}
}

// GetInt returns the integer value of the environment variable named by key.
// If the variable is unset or unparsable, defaultValue is returned.
func GetInt(key string, defaultValue int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultValue
	}
	var parsed int
	if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil {
		return defaultValue
	}
	return parsed
}
