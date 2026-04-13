// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return dispatch(args, commands(), stdout, stderr)
}

func commands() []*command {
	var cmds []*command
	cmds = []*command{
		{
			name:    "help",
			summary: "show this help message",
			run: func(_ *flag.FlagSet, stdout, _ io.Writer) int {
				printUsage(stdout, cmds)
				return 0
			},
		},
		{
			name:    "version",
			summary: "print version information",
			run: func(_ *flag.FlagSet, stdout, _ io.Writer) int {
				fmt.Fprintln(stdout, versionInfo())
				return 0
			},
		},
	}
	return cmds
}
