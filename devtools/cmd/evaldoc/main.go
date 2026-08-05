// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The evaldoc command takes a module_path@version or a local directory
// path, loads the module, and prints symbols and whether they have
// documentation to standard output.
//
// It can be used to better understand the "documentation coverage" score
// on a package's evaluations page (pkg.go.dev/IMPORT/PATH?tab=evals).
// Run it on your module to get a list of symbols which need documentation,
// and whether they have documentation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/proxy"
)

var (
	proxyURL = flag.String("proxy", "", "module proxy URL (defaults to GOPROXY or https://proxy.golang.org)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] (module_path@version | local_dir_path)\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "local_dir_path must start with one of: / . ~")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	arg := flag.Arg(0)
	modulePath, contentDir, err := getContentDir(arg)
	if err != nil {
		log.Fatal(err)
	}

	if err := evalDoc(modulePath, contentDir); err != nil {
		log.Fatal(err)
	}
}

// resolveLocalPath expands a leading "~" to the user's home directory and returns
// the absolute path of arg.
func resolveLocalPath(arg string) (string, error) {
	if strings.HasPrefix(arg, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("arg is local path, but cannot get home directory: %v", err)
		}
		arg = filepath.Join(home, arg[1:])
	}
	return filepath.Abs(arg)
}

// getContentDir uses arg to find a module, and returns an fs.FS whose root is the
// content directory of that module (the directory containing the go.mod file).
// It also returns the module path.
func getContentDir(arg string) (modulePath string, contentDir fs.FS, err error) {
	if len(arg) == 0 {
		return "", nil, errors.New("empty argument")
	}
	if arg[0] == '/' || arg[0] == '.' || arg[0] == '~' {
		localPath, err := resolveLocalPath(arg)
		if err != nil {
			return "", nil, fmt.Errorf("failed to resolve path %s: %w", arg, err)
		}
		fi, err := os.Stat(localPath)
		if err != nil || !fi.IsDir() {
			return "", nil, fmt.Errorf("%s is not a directory", localPath)
		}
		modBytes, err := os.ReadFile(filepath.Join(localPath, "go.mod"))
		if err != nil {
			return "", nil, fmt.Errorf("failed to read go.mod in %s: %w", localPath, err)
		}
		modulePath = modfile.ModulePath(modBytes)
		if modulePath == "" {
			return "", nil, fmt.Errorf("go.mod in %s contains no module path", localPath)
		}
		return modulePath, os.DirFS(localPath), nil
	}

	var reqVer string
	var found bool
	modulePath, reqVer, found = strings.Cut(arg, "@")
	if !found || reqVer == "" {
		reqVer = "latest"
	}

	pURL := *proxyURL
	if pURL == "" {
		pURL = os.Getenv("GOPROXY")
	}
	if pURL == "off" {
		return "", nil, errors.New("GOPROXY is off")
	}
	if pURL == "" {
		pURL = "https://proxy.golang.org"
	}
	// Take the first URL from the GOPROXY list.
	if idx := strings.IndexAny(pURL, ",|"); idx != -1 {
		pURL = pURL[:idx]
	}

	ctx := context.Background()
	proxyClient, err := proxy.New(pURL, http.DefaultTransport)
	if err != nil {
		return "", nil, fmt.Errorf("proxy.New(%q): %w", pURL, err)
	}
	proxyClient = proxyClient.WithFetchDisabled()

	verInfo, err := proxyClient.Info(ctx, modulePath, reqVer)
	if err != nil {
		return "", nil, fmt.Errorf("proxyClient.Info(%q, %q): %w", modulePath, reqVer, err)
	}
	resolvedVersion := verInfo.Version

	zipReader, err := proxyClient.Zip(ctx, modulePath, resolvedVersion)
	if err != nil {
		return "", nil, fmt.Errorf("proxyClient.Zip(%q, %q): %w", modulePath, resolvedVersion, err)
	}

	contentDir, err = fs.Sub(zipReader, modulePath+"@"+resolvedVersion)
	if err != nil {
		return "", nil, fmt.Errorf("fs.Sub: %w", err)
	}

	return modulePath, contentDir, nil
}

// packageDirs walks contentDir to find directories corresponding to valid import paths,
// and returns their relative paths.
func packageDirs(contentDir fs.FS) ([]string, error) {
	var dirs []string
	err := fs.WalkDir(contentDir, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// Skip directory trees under internal, vendor and testdata.
		name := d.Name()
		if name == "internal" || name == "vendor" || name == "testdata" {
			return fs.SkipDir
		}
		// Skip directory trees if the name starts with a ".", other than "." itself.
		if len(name) >= 2 && name[0] == '.' {
			return fs.SkipDir
		}
		dirs = append(dirs, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

func evalDoc(modulePath string, contentDir fs.FS) error {
	dirs, err := packageDirs(contentDir)
	if err != nil {
		return err
	}

	for _, relDir := range dirs {
		importPath := modulePath
		if relDir != "." && relDir != "" {
			importPath = path.Join(modulePath, relDir)
		}

		entries, err := fs.ReadDir(contentDir, relDir)
		if err != nil {
			return err
		}

		fset := token.NewFileSet()
		var astFiles []*ast.File

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fname := entry.Name()
			if !strings.HasSuffix(fname, ".go") || strings.HasSuffix(fname, "_test.go") {
				continue
			}
			fp := path.Join(relDir, fname)
			content, err := fs.ReadFile(contentDir, fp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: reading: %v", fp, err)
				continue
			}
			f, err := parser.ParseFile(fset, fname, content, parser.ParseComments)
			if err == nil {
				astFiles = append(astFiles, f)
			} else {
				fmt.Fprintf(os.Stderr, "%s: parsing: %v", fp, err)
			}
		}

		if len(astFiles) == 0 {
			continue
		}

		fmt.Println(importPath)
		frontend.CollectSymbols(astFiles, func(id *ast.Ident, has bool) {
			fmt.Printf("  %s: %t\n", id.Name, has)
		})
	}
	return nil
}
