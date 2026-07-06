// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"strings"
)

func resolveTarget(args []string) string {
	target := strings.ToLower(strings.TrimSpace(os.Getenv("TARGET")))
	if target == "" {
		target = "both"
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--target=") {
			if v := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--target="))); v != "" {
				target = v
			}
			continue
		}
		if arg == "--target" && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				target = strings.ToLower(next)
				i++
			}
		}
	}

	return target
}

func needsNATS(target string) bool {
	return target == "both" || target == "nats"
}

func needsOpenSearch(target string) bool {
	return target == "both" || target == "opensearch"
}
