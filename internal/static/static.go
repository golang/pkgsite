// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/evanw/esbuild doesn't compile on plan9
//go:build !plan9

// Package static builds static assets for the frontend and the worker.
package static

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"golang.org/x/pkgsite/internal/derrors"
)

// Build compiles TypeScript files into minified JavaScript
// files using github.com/evanw/esbuild.
//
// This function is used in Server.staticHandler with Watch=true
// when cmd/frontend is run in dev mode and in
// devtools/cmd/static/main.go with Watch=false for building
// productionized assets.
func Build(config Config) error {
	files, err := getFiles(config.EntryPoint, config.Bundle)
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := processFile(config, file); err != nil {
			return err
		}
	}
	return nil
}

func processFile(config Config, filename string) (err error) {
	defer derrors.Wrap(&err, "%s", filename)
	contents, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	license, err := extractLicense(contents)
	if err != nil {
		return err
	}
	license = normalizeLicense(license)
	var ext string
	if strings.HasSuffix(filename, ".ts") {
		ext = "js"
	} else if strings.HasSuffix(filename, ".css") {
		ext = "css"
	} else {
		return errors.New("unsupported filename type")
	}
	options := api.BuildOptions{
		EntryPoints:  []string{filename},
		Bundle:       config.Bundle,
		Outdir:       config.EntryPoint,
		Outbase:      config.EntryPoint,
		Write:        true,
		Platform:     api.PlatformBrowser,
		Format:       api.FormatESModule,
		OutExtension: map[string]string{".css": ".min.css"},
		External:     []string{"*.svg"},
		Banner:       map[string]string{ext: license},
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
		defer ctx.Dispose()
		if err := ctx.Watch(api.WatchOptions{}); err != nil {
			return err
		}
	} else {
		result := api.Build(options)
		if len(result.Errors) > 0 {
			return fmt.Errorf("error building: %v", result.Errors)
		}
		if len(result.Warnings) > 0 {
			return fmt.Errorf("warning building: %v", result.Warnings)
		}
	}
	return nil
}

// getFiles walks the given directory and collects entry file paths
// for esbuild. It ignores test files and files prefixed with an underscore.
// Underscore prefixed files are assumed to be imported by and bundled together
// with the output of an entry file.
func getFiles(dir string, bundle bool) ([]string, error) {
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

// Match a /* ... */ comment starting at the beginning of the input.
// The (?s:...) syntax matches over multiple lines.
// match[1] contains the comment text with its close delimeter.
var reBlock = regexp.MustCompile(`(?s:^\s*/\*[*!]?(.*?\*/))`)

// extractLicense takes a []byte and returns the contents of the license comment in
// it. The comment must be of the form /*...*/ at the start of the file.
// The returned string includes the close comment delimiter but not the open one.
func extractLicense(contents []byte) (string, error) {
	match := reBlock.FindSubmatch(contents)
	if match == nil {
		return "", errors.New("no initial /*...*/ comment")
	}
	comment := string(match[1])
	if !strings.Contains(strings.ToLower(comment), "copyright") {
		return "", errors.New("initial comment missing 'copyright'")
	}
	return comment, nil
}

func normalizeLicense(lic string) string {
	// TS files have a "@license" line which we want to remove.
	return "/*!" + strings.Replace(lic, " * @license\n", "", 1)
}
