// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package commands

import (
	"errors"
	"flag"
	"fmt"
	"strings"
)

// NewFlagSet returns the FlagSet for a subcommand. usage is the invocation
// shown in --help after "project-cli", e.g. "sync reindex-projects [flags]".
func NewFlagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: project-cli %s\n\nflags:\n", usage)
		fs.PrintDefaults()
	}
	return fs
}

// ParseFlags parses args into fs. When --help was requested, the usage has
// already been printed and help is true: the caller should return nil.
func ParseFlags(fs *flag.FlagSet, args []string) (help bool, err error) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// RejectExtraArgs fails when positional arguments remain after flag parsing,
// for subcommands that take none.
func RejectExtraArgs(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return nil
}
