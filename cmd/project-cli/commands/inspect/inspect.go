// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package inspect holds the read-only diagnostic commands. None of them writes
// to NATS, OpenSearch or OpenFGA; each exits non-zero when it finds a problem so
// it can gate a job.
package inspect

import (
	"encoding/json"
	"fmt"
	"io"

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

// problemsError turns a non-zero finding count into the run's error.
func problemsError(name string, n int) error {
	if n == 0 {
		return nil
	}
	return fmt.Errorf("%s found %d problem(s); see report", name, n)
}
