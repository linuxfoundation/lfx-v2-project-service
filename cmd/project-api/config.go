// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// cmdFlags holds the command-line flags for the project service.
type cmdFlags struct {
	Debug bool
	Port  string
	Bind  string
}

func parseFlags(defaultPort string) cmdFlags {
	flag.Usage = func() {
		flag.PrintDefaults()
		os.Exit(2)
	}

	// flag.CommandLine uses flag.ExitOnError, so a parse failure exits here.
	flags, err := parseFlagSet(flag.CommandLine, os.Args[1:], defaultPort)
	if err != nil {
		os.Exit(2)
	}

	// Based on the debug flag, set the log level environment variable used by [log.InitStructureLogConfig]
	if flags.Debug {
		err := os.Setenv("LOG_LEVEL", "debug")
		if err != nil {
			slog.With(constants.ErrKey, err).Error("error setting log level")
			os.Exit(1)
		}
	}

	return flags
}

// parseFlagSet defines the command-line flags on fs and parses args.
func parseFlagSet(fs *flag.FlagSet, args []string, defaultPort string) (cmdFlags, error) {
	var debug = fs.Bool("d", false, "enable debug logging")
	var port = fs.String("p", defaultPort, "listen port")
	var bind = fs.String("bind", "*", "interface to bind on")

	if err := fs.Parse(args); err != nil {
		return cmdFlags{}, err
	}

	return cmdFlags{
		Debug: *debug,
		Port:  *port,
		Bind:  *bind,
	}, nil
}

// environment holds the environment variables for the project service.
type environment struct {
	NatsURL             string
	Port                string
	SkipEtagValidation  bool
	LFXSelfServeBaseURL string
	EmailsEnabled       bool
	InvitesEnabled      bool
	FGAEnabled          bool
	// ArchivedDissolutionDateRequiredFrom is the raw, trimmed
	// ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM value; it is parsed by
	// archivedDissolutionDateRequiredFrom once logging is configured.
	ArchivedDissolutionDateRequiredFrom string
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
		FGAEnabled:          os.Getenv("FGA_ENABLED") == "true",

		ArchivedDissolutionDateRequiredFrom: strings.TrimSpace(os.Getenv("ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM")),
	}
}

// archivedDissolutionDateRequiredFrom parses the temporary LFXV2-1733 waiver
// date (YYYY-MM-DD, UTC) from ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM and logs
// whether the waiver is active at now. Until that date, Archived projects may
// be saved without an entity dissolution date (legacy v1 imports); from that
// date on the rule is enforced again, so the waiver ends without a code change.
// An empty value returns the zero time (rule enforced). An invalid value is
// logged as an error and also returns the zero time, so the service fails
// closed rather than crashing.
func archivedDissolutionDateRequiredFrom(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	requiredFrom, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("invalid ARCHIVED_DISSOLUTION_DATE_REQUIRED_FROM; archived dissolution date rule is enforced",
			"value", raw)
		return time.Time{}
	}
	if now.UTC().Before(requiredFrom) {
		slog.Info("archived dissolution date rule waived until required-from date (temporary LFXV2-1733 waiver)",
			"required_from", requiredFrom.Format(time.DateOnly))
	} else {
		slog.Info("archived dissolution date waiver has expired; rule is enforced",
			"required_from", requiredFrom.Format(time.DateOnly))
	}
	return requiredFrom
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
