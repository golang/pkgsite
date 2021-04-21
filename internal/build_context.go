// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "fmt"

// A BuildContext describes a build context for the Go tool: information needed
// to build a Go package. For our purposes, we only care about the information
// that affects documentation generated from the package.
type BuildContext struct {
	GOOS, GOARCH string
}

// String returns a string formatted representation of the build context.
func (b BuildContext) String() string {
	return fmt.Sprintf("%s/%s", b.GOOS, b.GOARCH)
}

// Match reports whether its receiver, which acts like a pattern, matches its
// target, an ordinary BuildContext. In addition to the usual values, a pattern
// can have an empty GOOS or GOARCH, which means "match anything."
func (pattern BuildContext) Match(target BuildContext) bool {
	match := func(pat, targ string) bool { return pat == "" || targ == All || pat == targ }

	return match(pattern.GOOS, target.GOOS) && match(pattern.GOARCH, target.GOARCH)
}

// All represents all values for a build context element (GOOS or GOARCH).
const All = "all"

var (
	BuildContextAll     = BuildContext{All, All}
	BuildContextLinux   = BuildContext{"linux", "amd64"}
	BuildContextWindows = BuildContext{"windows", "amd64"}
	BuildContextDarwin  = BuildContext{"darwin", "amd64"}
	BuildContextJS      = BuildContext{"js", "wasm"}
)

// BuildContexts are the build contexts we check when loading a package (see
// internal/fetch/load.go).
// We store documentation for all of the listed contexts.
// The order determines which environment's docs we will show as the default.
var BuildContexts = []BuildContext{
	BuildContextLinux,
	BuildContextWindows,
	BuildContextDarwin,
	BuildContextJS,
}

// CompareBuildContexts returns a negative number, 0, or a positive number depending on
// the relative positions of c1 and c2 in BuildContexts.
func CompareBuildContexts(c1, c2 BuildContext) int {
	if c1 == c2 {
		return 0
	}
	// Although we really shouldn't see a BuildContext with "all" here, we may if the
	// DB erroneously has both an all/all row and some other row. So just prefer the all/all.
	if c1 == BuildContextAll {
		if c2 == BuildContextAll {
			return 0
		}
		return -1
	}
	if c2 == BuildContextAll {
		return 1
	}
	pos := func(c BuildContext) int {
		for i, d := range BuildContexts {
			if c == d {
				return i
			}
		}
		return len(BuildContexts) // unknowns sort last
	}
	return pos(c1) - pos(c2)
}

// BuildContext returns the BuildContext for d.
func (d *Documentation) BuildContext() BuildContext {
	return BuildContext{GOOS: d.GOOS, GOARCH: d.GOARCH}
}

// DocumentationForBuildContext returns the first Documentation the list that
// matches the BuildContext, or nil if none does. A Documentation matches if its
// GOOS and GOARCH fields are the same as those of the BuildContext, or if the
// Documentation field is "all", or if the BuildContext field is empty. That is,
// empty BuildContext fields act as wildcards. So the zero BuildContext will
// match the first element of docs, if there is one.
func DocumentationForBuildContext(docs []*Documentation, bc BuildContext) *Documentation {
	for _, d := range docs {
		if bc.Match(BuildContext{d.GOOS, d.GOARCH}) {
			return d
		}
	}
	return nil
}
