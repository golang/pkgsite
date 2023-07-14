// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static

type Config struct {
	// Entrypoint is a directory in which to build TypeScript
	// sources.
	EntryPoint string

	// Bundle is true if files imported by an entry file
	// should be joined together in a single output file.
	Bundle bool

	// Watch is true in development. Sourcemaps are placed inline,
	// the output is unminified, and changes to any TypeScript
	// files will force a rebuild of the JavaScript output.
	Watch bool
}
