// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"os"
	"strings"
)

func resolveTarget(args []string) string {
	target := strings.ToLower(strings.TrimSpace(os.Getenv("TARGET")))
	if target == "" {
		target = "both"
	}

	fs := flag.NewFlagSet("target", flag.ContinueOnError)
	t := fs.String("target", target, "")
	_ = fs.Parse(args)
	if parsed := strings.ToLower(strings.TrimSpace(*t)); parsed != "" {
		target = parsed
	}
	return target
}

func needsNATS(target string) bool {
	return target == "both" || target == "nats"
}

func needsOpenSearch(target string) bool {
	return target == "both" || target == "opensearch"
}
