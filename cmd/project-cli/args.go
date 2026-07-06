// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import "strings"

type parsedArgs struct {
	Positionals []string
	SubArgs     []string
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
