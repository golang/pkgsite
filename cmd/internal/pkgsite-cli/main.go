// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command pkgsite-cli queries the pkg.go.dev API for information about
// Go packages and modules.
//
// Usage:
//
//	pkgsite-cli package <package>[@version] [flags]  package information
//	pkgsite-cli module <module>[@version] [flags]      module information
//	pkgsite-cli search <query> [flags]                 search for packages
//
// See doc/pkgsite-cli.md for the full design document.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return dispatch(args, commands(), stdout, stderr)
}

func commands() []*command {
	var pf packageFlags
	pkgFS := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	pf.register(pkgFS)

	var mf moduleFlags
	modFS := flag.NewFlagSet(filepath.Base(os.Args[0])+" module", flag.ContinueOnError)
	mf.register(modFS)

	var sf searchFlags
	searchFS := flag.NewFlagSet(filepath.Base(os.Args[0])+" search", flag.ContinueOnError)
	sf.register(searchFS)

	pkgRun := func(fs *flag.FlagSet, stdout, stderr io.Writer) int { return runPackage(fs, &pf, stdout, stderr) }

	var cmds []*command
	cmds = []*command{
		{
			name:    "package",
			args:    "<package>[@version]",
			summary: "package information",
			flags:   pkgFS,
			run:     pkgRun,
		},
		{
			name:    "module",
			args:    "<module>[@version]",
			summary: "module information",
			flags:   modFS,
			run:     func(fs *flag.FlagSet, stdout, stderr io.Writer) int { return runModule(fs, &mf, stdout, stderr) },
		},
		{
			name:    "search",
			args:    "<query>",
			summary: "search for packages",
			flags:   searchFS,
			run:     func(fs *flag.FlagSet, stdout, stderr io.Writer) int { return runSearch(fs, &sf, stdout, stderr) },
		},
		{
			name:    "help",
			summary: "show this help message",
			run:     func(_ *flag.FlagSet, stdout, _ io.Writer) int { printUsage(stdout, cmds); return 0 },
		},
		{
			name:    "version",
			summary: "print version information",
			run:     func(_ *flag.FlagSet, stdout, _ io.Writer) int { fmt.Fprintln(stdout, versionInfo()); return 0 },
		},
	}
	return cmds
}

// splitPathVersion splits "path@version" into its components.
// If there is no @, version is empty.
func splitPathVersion(s string) (path, version string) {
	path, version, _ = strings.Cut(s, "@")
	return path, version
}

// handleErr writes an error message. In JSON mode, the error is written
// to stdout as a JSON object so callers can parse it. In text mode, it
// goes to stderr.
func handleErr(stdout, stderr io.Writer, err error, jsonMode bool) int {
	if jsonMode {
		aerr, ok := err.(*apiError)
		if !ok {
			aerr = &apiError{Code: 1, Message: err.Error()}
		}
		writeJSON(stdout, stderr, aerr)
		return 1
	}
	fmt.Fprintln(stderr, err)
	return 1
}

func writeJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
