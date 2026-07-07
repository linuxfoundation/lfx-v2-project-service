// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import "strings"

// parsedArgs holds the result of splitting raw CLI arguments into positionals
// (command, subcommand) and subcommand args (everything else).
type parsedArgs struct {
	// Positionals contains the non-flag arguments up to positionalLimit (command, subcommand).
	Positionals []string
	// SubArgs contains everything after both positionals, forwarded as-is to
	// the subcommand's own FlagSet.
	SubArgs []string
}

func splitArgs(args []string, positionalLimit int) parsedArgs {
	var result parsedArgs
	for _, arg := range args {
		if len(result.Positionals) >= positionalLimit {
			result.SubArgs = append(result.SubArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") {
			result.SubArgs = append(result.SubArgs, arg)
			continue
		}
		result.Positionals = append(result.Positionals, arg)
	}
	return result
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
