// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package inspect holds the read-only diagnostic commands. None of them writes
// to NATS, OpenSearch or OpenFGA; each exits non-zero when it finds a problem so
// it can gate a job.
package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
)

type command struct{}

func (c *command) Name() string { return "inspect" }

func (c *command) Help() string {
	return "read-only diagnostics across the project KV stores, OpenSearch and OpenFGA"
}

func (c *command) Subcommands() map[string]commands.Subcommand {
	return commands.IndexSubcommands(
		&projectSubcommand{},
		&slugIndexSubcommand{},
		&orphansSubcommand{},
		&indexDriftSubcommand{},
		&settingsAccessSubcommand{},
		&childrenSubcommand{},
		&parentsSubcommand{},
	)
}

// NewCommand creates the inspect command group.
func NewCommand() commands.Command {
	return &command{}
}

// finding is one problem a diagnostic found.
type finding struct {
	Kind   string `json:"kind"`
	Slug   string `json:"slug,omitempty"`
	UID    string `json:"uid,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// writeReport prints v as indented JSON.
func writeReport(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// Report statuses. Every report carries one, so an empty findings list reads as a
// result and not as a missing answer.
const (
	statusOK       = "ok"
	statusProblems = "problems_found"
)

// reportStatus is embedded first in every report.
type reportStatus struct {
	Status string `json:"status"`
}

func statusFor(problems int) reportStatus {
	if problems == 0 {
		return reportStatus{Status: statusOK}
	}
	return reportStatus{Status: statusProblems}
}

// finishReport prints the report, logs a one-line verdict, and returns the run's
// error: nil for a clean run and an error naming the problem count otherwise.
// scanned says how much was looked at, so "no problems" is not read as "nothing checked".
func finishReport(ctx context.Context, name string, out io.Writer, report any, problems int, scanned string) error {
	if err := writeReport(out, report); err != nil {
		return err
	}
	if problems == 0 {
		slog.InfoContext(ctx, fmt.Sprintf("%s: OK (%s, no problems)", name, scanned))
		return nil
	}
	slog.WarnContext(ctx, fmt.Sprintf("%s: %d problem(s) found (%s)", name, problems, scanned))
	return problemsError(name, problems)
}

// problemsError turns a non-zero finding count into the run's error.
func problemsError(name string, n int) error {
	if n == 0 {
		return nil
	}
	return fmt.Errorf("%s found %d problem(s); see report", name, n)
}
