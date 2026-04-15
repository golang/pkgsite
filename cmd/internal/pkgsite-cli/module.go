// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io"
)

func runModule(fs *flag.FlagSet, m *moduleFlags, stdout, stderr io.Writer) int {
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	path, version := splitPathVersion(fs.Arg(0))

	c := newClient(m.server)
	mod, err := c.getModule(path, version, m)
	if err != nil {
		return handleErr(stdout, stderr, err, m.jsonOut)
	}
	result := moduleResult{Module: mod}

	// TODO: run concurrently ?
	if m.versions {
		vers, err := c.getVersions(path, m)
		if err != nil {
			return handleErr(stdout, stderr, err, m.jsonOut)
		}
		result.Versions = vers
	}
	if m.vulns {
		vulns, err := c.getVulns(path, version, m)
		if err != nil {
			return handleErr(stdout, stderr, err, m.jsonOut)
		}
		result.Vulns = vulns
	}
	if m.packages {
		pkgs, err := c.getPackages(path, version, m)
		if err != nil {
			return handleErr(stdout, stderr, err, m.jsonOut)
		}
		result.Packages = pkgs
	}

	if m.jsonOut {
		return writeJSON(stdout, stderr, result)
	}
	formatModule(stdout, result)
	return 0
}

// moduleFlags are flags for the module subcommand.
type moduleFlags struct {
	commonFlags
	readme   bool
	licenses bool
	versions bool
	vulns    bool
	packages bool
}

func (f *moduleFlags) register(fs *flag.FlagSet) {
	f.commonFlags.register(fs)
	fs.BoolVar(&f.readme, "readme", false, "include README")
	fs.BoolVar(&f.licenses, "licenses", false, "show license information")
	fs.BoolVar(&f.versions, "versions", false, "list versions")
	fs.BoolVar(&f.vulns, "vulns", false, "list vulnerabilities")
	fs.BoolVar(&f.packages, "packages", false, "list packages")
}
