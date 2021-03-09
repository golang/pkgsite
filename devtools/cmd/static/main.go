// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command static compiles TypeScript files into minified
// JavaScript files and creates linked sourcemaps.
package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"

	"golang.org/x/pkgsite/internal/static"
)

var check = flag.Bool("check", false, "disable write mode and check that output files are valid")

func main() {
	flag.Parse()

	staticPath := "content/static"
	result, err := static.Build(static.Config{StaticPath: staticPath, Write: !*check})
	if err != nil {
		log.Fatal(err)
	}

	if *check {
		for _, v := range result.OutputFiles {
			file, err := ioutil.ReadFile(v.Path)
			if err != nil {
				log.Fatal(err)
			}
			if bytes.Equal(file, v.Contents) {
				log.Fatalf("static files out of sync, try running 'go run ./devtools/cmd/static'")
			}
		}
	}
}
