// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/evanw/esbuild doesn't compile on plan9
//go:build !plan9

// Package static builds static assets for the frontend and the worker.
package static

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Build compiles TypeScript files into minified JavaScript
// files using github.com/evanw/esbuild.
//
// This function is used in Server.staticHandler with Watch=true
// when cmd/frontend is run in dev mode and in
// devtools/cmd/static/main.go with Watch=false for building
// productionized assets.
func Build(config Config) error {
	files, err := getEntry(config.EntryPoint, config.Bundle)
	if err != nil {
		return err
	}
	options := api.BuildOptions{
		EntryPoints:  files,
		Bundle:       config.Bundle,
		Outdir:       config.EntryPoint,
		Write:        true,
		Platform:     api.PlatformBrowser,
		Format:       api.FormatESModule,
		OutExtension: map[string]string{".css": ".min.css"},
		External:     []string{"*.svg"},
		Banner: map[string]string{"css": "/*!\n" +
			" * Copyright 2021 The Go Authors. All rights reserved.\n" +
			" * Use of this source code is governed by a BSD-style\n" +
			" * license that can be found in the LICENSE file.\n" +
			" */"},
	}
	options.MinifyIdentifiers = true
	options.MinifySyntax = true
	options.MinifyWhitespace = true
	options.Sourcemap = api.SourceMapLinked
	if config.Watch {
		ctx, err := api.Context(options)
		if err != nil {
			return err
		}
		return ctx.Watch(api.WatchOptions{})
	}
	result := api.Build(options)
	if len(result.Errors) > 0 {
		return fmt.Errorf("error building static files: %v", result.Errors)
	}
	if len(result.Warnings) > 0 {
		return fmt.Errorf("error building static files: %v", result.Warnings)
	}
	return nil
}

// getEntry walks the given directory and collects entry file paths
// for esbuild. It ignores test files and files prefixed with an underscore.
// Underscore prefixed files are assumed to be imported by and bundled together
// with the output of an entry file.
func getEntry(dir string, bundle bool) ([]string, error) {
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
		isTS := strings.HasSuffix(basePath, ".ts")
		isCSS := strings.HasSuffix(basePath, ".css") && !strings.HasSuffix(basePath, ".min.css")
		if notPartial && notTest && (isTS || (bundle && isCSS)) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}
