// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

// A BuildContext describes a build context for the Go tool: information needed
// to build a Go package. For our purposes, we only care about the information
// that affects documentation generated from the package.
type BuildContext struct {
	GOOS, GOARCH string
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
	// We should never see a BuildContext with "all" here.
	if c1.GOOS == All || c1.GOARCH == All || c2.GOOS == All || c2.GOARCH == All {
		panic("BuildContext with 'all'")
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
		if (bc.GOOS == "" || d.GOOS == All || bc.GOOS == d.GOOS) && (bc.GOARCH == "" || d.GOARCH == All || bc.GOARCH == d.GOARCH) {
			return d
		}
	}
	return nil
}
