// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
)

// A command describes a subcommand of the tool.
type command struct {
	name    string // subcommand name
	args    string // e.g. "<package>[@version]"; empty for no-arg commands
	summary string // one-line description
	flags   *flag.FlagSet
	run     func(fs *flag.FlagSet, stdout, stderr io.Writer) int
}

func (c *command) usageLine() string {
	parts := []string{filepath.Base(os.Args[0]), c.name}
	if c.args != "" {
		parts = append(parts, "[flags]", c.args)
	}
	return strings.Join(parts, " ")
}

// printCommandUsage writes usage for a single command to w.
func printCommandUsage(w io.Writer, c *command) {
	fmt.Fprintf(w, "Usage: %s\n", c.usageLine())
	fmt.Fprintf(w, "\n%s\n", c.summary)
	if c.flags != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Flags:")
		c.flags.SetOutput(w)
		c.flags.PrintDefaults()
	}
}

// printUsage writes usage for all commands to w.
func printUsage(w io.Writer, cmds []*command) {
	fmt.Fprintln(w, "Usage:")
	for _, c := range cmds {
		line := c.usageLine()
		fmt.Fprintf(w, "  %-50s %s\n", line, c.summary)
	}
	fmt.Fprintf(w, "\nRun \"%s <command> -h\" for command-specific flags.\n", filepath.Base(os.Args[0]))
}

// dispatch finds and runs the matching command. It returns the exit code.
func dispatch(args []string, cmds []*command, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr, cmds)
		return 2
	}

	// Bare "-h" with no positional arg: show overview.
	hasPositional := false
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			hasPositional = true
			break
		}
	}
	if !hasPositional {
		printUsage(stderr, cmds)
		for _, a := range args {
			if a == "-h" || a == "-help" || a == "--help" {
				return 0
			}
		}
		return 2
	}

	named := map[string]*command{}
	for _, c := range cmds {
		named[c.name] = c
	}

	// Scan for the first positional arg to determine the subcommand.
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if c, ok := named[a]; ok {
			rest := slices.Delete(args, i, i+1)
			return parseAndRun(c, rest, stdout, stderr)
		}
		fmt.Fprintf(stderr, "unknown command: %s\n", a)
		printUsage(stderr, cmds)
		return 2
	}

	printUsage(stderr, cmds)
	return 2
}

func parseAndRun(c *command, args []string, stdout, stderr io.Writer) int {
	if c.flags == nil {
		// No-arg command (help, version).
		for _, a := range args {
			if a == "-h" || a == "-help" || a == "--help" {
				printCommandUsage(stderr, c)
				return 0
			}
		}
		return c.run(nil, stdout, stderr)
	}
	c.flags.SetOutput(stderr)
	c.flags.Usage = func() { printCommandUsage(stderr, c) }
	// TODO: Consider supporting flags after positional arguments for better UX.
	// Currently, flags must appear before positional arguments.
	// Works: pkgsite-cli package -doc=text -examples -imports -json -module golang.org/x/tools golang.org/x/tools/go/packages
	// Fails: pkgsite-cli package golang.org/x/tools/go/packages -doc=text -examples -imports -json -module golang.org/x/tools
	if err := c.flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	return c.run(c.flags, stdout, stderr)
}

func versionInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return filepath.Base(os.Args[0]) + " (unknown version)"
	}
	v := bi.Main.Version
	if v == "" || v == "(devel)" {
		const vcsRevisionLen = 12
		v = "devel"
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= vcsRevisionLen {
				v += " " + s.Value[:vcsRevisionLen]
			}
		}
	}
	return filepath.Base(os.Args[0]) + " " + v + " " + bi.GoVersion
}

const defaultServer = "https://pkg.go.dev"

// commonFlags are shared across all subcommands.
type commonFlags struct {
	jsonOut bool
	limit   int
	token   string
	server  string
}

func (f *commonFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.jsonOut, "json", false, "output JSON")
	fs.IntVar(&f.limit, "limit", 0, "max results (default: 20 text, 100 json)")
	fs.StringVar(&f.token, "token", "", "pagination token (JSON mode)")
	fs.StringVar(&f.server, "server", defaultServer, "API server URL")
}

func (f *commonFlags) effectiveLimit() int {
	if f.limit > 0 {
		return f.limit
	}
	if f.jsonOut {
		return 100
	}
	return 20
}
