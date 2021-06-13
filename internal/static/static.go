// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package static builds static assets for the frontend and the worker.
package static

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

type Config struct {
	// Entrypoint is a directory in which to to build TypeScript
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

// Build compiles TypeScript files into minified JavaScript
// files using github.com/evanw/esbuild.
//
// This function is used in Server.staticHandler with Watch=true
// when cmd/frontend is run in dev mode and in
// devtools/cmd/static/main.go with Watch=false for building
// productionized assets.
func Build(config Config) (*api.BuildResult, error) {
	files, err := getEntry(config.EntryPoint)
	if err != nil {
		return nil, err
	}
	options := api.BuildOptions{
		EntryPoints: files,
		Bundle:      config.Bundle,
		Outdir:      config.EntryPoint,
		Write:       true,
	}
	if config.Watch {
		options.Sourcemap = api.SourceMapInline
		options.Watch = &api.WatchMode{}
	} else {
		options.MinifyIdentifiers = true
		options.MinifySyntax = true
		options.MinifyWhitespace = true
		options.Sourcemap = api.SourceMapLinked
	}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("error building static files: %v", result.Errors)
	}
	if len(result.Warnings) > 0 {
		return nil, fmt.Errorf("error building static files: %v", result.Warnings)
	}
	return &result, nil
}

// getEntry walks the the given directory and collects entry file paths
// for esbuild. It ignores test files and files prefixed with an underscore.
// Underscore prefixed files are assumed to be imported by and bundled together
// with the output of an entry file.
func getEntry(dir string) ([]string, error) {
	var matches []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		basePath := filepath.Base(path)
		notPartial := !strings.HasPrefix(basePath, "_")
		notTest := !strings.HasSuffix(basePath, ".test.ts")
		matched, err := filepath.Match("*.ts", basePath)
		if err != nil {
			return err
		}
		if notPartial && notTest && matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}
