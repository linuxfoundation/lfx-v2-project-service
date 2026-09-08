// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// cmdFlags holds the command-line flags for the project service.
type cmdFlags struct {
	Debug bool
	Port  string
	Bind  string
}

func parseFlags(defaultPort string) cmdFlags {
	var debug = flag.Bool("d", false, "enable debug logging")
	var port = flag.String("p", defaultPort, "listen port")
	var bind = flag.String("bind", "*", "interface to bind on")

	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	// Based on the debug flag, set the log level environment variable used by [log.InitStructureLogConfig]
	if *debug {
		err := os.Setenv("LOG_LEVEL", "debug")
		if err != nil {
			slog.With(constants.ErrKey, err).Error("error setting log level")
			os.Exit(1)
		}
	}

	return cmdFlags{
		Debug: *debug,
		Port:  *port,
		Bind:  *bind,
	}
}

// environment holds the environment variables for the project service.
type environment struct {
	NatsURL             string
	Port                string
	SkipEtagValidation  bool
	LFXSelfServeBaseURL string
	EmailsEnabled       bool
	InvitesEnabled      bool
}

func parseEnv() environment {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	skipEtagValidation := false
	skipEtagValidationStr := os.Getenv("SKIP_ETAG_VALIDATION")
	if skipEtagValidationStr == "true" {
		skipEtagValidation = true
	}
	lfxSelfServeBaseURL := LFXSelfServeBaseURL()
	return environment{
		NatsURL:             natsURL,
		Port:                port,
		SkipEtagValidation:  skipEtagValidation,
		LFXSelfServeBaseURL: lfxSelfServeBaseURL,
		EmailsEnabled:       os.Getenv("EMAILS_ENABLED") == "true",
		InvitesEnabled:      os.Getenv("INVITES_ENABLED") == "true",
	}
}

// LFXSelfServeBaseURL derives the LFX Self-Serve base URL from environment variables.
// LFX_SELF_SERVE_BASE_URL takes precedence; otherwise it falls back to LFX_ENVIRONMENT.
// When LFX_ENVIRONMENT is unset or unrecognized, prod is assumed (safe default for deployed environments).
func LFXSelfServeBaseURL() string {
	if url := os.Getenv("LFX_SELF_SERVE_BASE_URL"); url != "" {
		return url
	}
	switch os.Getenv("LFX_ENVIRONMENT") {
	case "prod", "production":
		return "https://app.lfx.dev"
	case "staging", "stg", "stage":
		return "https://app.staging.lfx.dev"
	case "dev", "development":
		return "https://app.dev.lfx.dev"
	default:
		return "https://app.lfx.dev"
	}
}
