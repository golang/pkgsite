// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command static compiles TypeScript files into minified
// JavaScript files and creates linked sourcemaps.
package main

import (
	"log"

	"golang.org/x/pkgsite/internal/static"
)

func main() {
	if err := static.Build(static.Config{EntryPoint: "static/frontend", Bundle: true}); err != nil {
		log.Fatal(err)
	}

	if err := static.Build(static.Config{EntryPoint: "static/worker", Bundle: true}); err != nil {
		log.Fatal(err)
	}
}
