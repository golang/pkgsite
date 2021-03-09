// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package static builds static assets for the frontend and the worker.
package static

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Build compiles TypeScript files into minified JavaScript
// files using github.com/evanw/esbuild. When run with watch=true
// sourcemaps are placed inline, the output is unminified, and
// changes to any TypeScript files will force a rebuild of the
// JavaScript output.
//
// This function is used in Server.staticHandler with watch=true
// when cmd/frontend is run in dev mode and in
// devtools/cmd/static/main.go with watch=false for building
// productionized assets.
func Build(staticPath string, watch bool) error {
	var entryPoints []string
	scriptDir := staticPath + "/js"
	files, err := ioutil.ReadDir(scriptDir)
	if err != nil {
		return err
	}
	for _, v := range files {
		if strings.HasSuffix(v.Name(), ".ts") && !strings.HasSuffix(v.Name(), ".test.ts") {
			entryPoints = append(entryPoints, scriptDir+"/"+v.Name())
		}
	}
	options := api.BuildOptions{
		EntryPoints: entryPoints,
		Outdir:      scriptDir,
		Write:       true,
	}
	if watch {
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
		return fmt.Errorf("error building static files: %v", result.Errors)
	}
	if len(result.Warnings) > 0 {
		return fmt.Errorf("error building static files: %v", result.Warnings)
	}
	return nil
}
