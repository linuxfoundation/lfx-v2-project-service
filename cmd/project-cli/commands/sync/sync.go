// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"

type command struct{}

func (c *command) Name() string { return "sync" }

func (c *command) Help() string {
	return "operational sync and migration jobs for project-service data stores"
}

func (c *command) Subcommands() map[string]commands.Subcommand {
	return map[string]commands.Subcommand{
		"rename-project-slug": &renameProjectSlugSubcommand{},
	}
}

// NewCommand creates the sync command group.
func NewCommand() commands.Command {
	return &command{}
}
