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

// BuildContexts are the build contexts we check when loading a package (see
// internal/fetch/load.go).
// We store documentation for all of the listed contexts.
// The order determines which environment's docs we will show as the default.
var BuildContexts = []BuildContext{
	{"linux", "amd64"},
	{"windows", "amd64"},
	{"darwin", "amd64"},
	{"js", "wasm"},
}

// CompareBuildContexts returns a negative number, 0, or a positive number depending on
// the relative positions of c1 and c2 in BuildContexts.
func CompareBuildContexts(c1, c2 BuildContext) int {
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
// BuildContext field is empty. That is, empty BuildContext fields act as
// wildcards. So the zero BuildContext will match the first element of docs, if
// there is one.
func DocumentationForBuildContext(docs []*Documentation, bc BuildContext) *Documentation {
	for _, d := range docs {
		if (bc.GOOS == "" || bc.GOOS == d.GOOS) && (bc.GOARCH == "" || bc.GOARCH == d.GOARCH) {
			return d
		}
	}
	return nil
}
