// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Pkgsite extracts and generates documentation for Go programs.
// It runs as a web server and presents the documentation as a
// web page.
//
// To install, run `go install ./cmd/pkgsite` from the pkgsite repo root.
//
// With no arguments, pkgsite will serve docs for the module in the current
// directory, which must have a go.mod file:
//
//	cd ~/repos/cue && pkgsite
//
// This form will also serve all of the module's required modules at their
// required versions. You can disable serving the required modules by passing
// -list=false.
//
// You can also serve docs from your module cache, directly from the proxy
// (it uses the GOPROXY environment variable), or both:
//
//	pkgsite -cache -proxy
//
// With either -cache or -proxy, pkgsite won't look for a module in the current
// directory. You can still provide modules on the local filesystem by listing
// their paths:
//
//	pkgsite -cache -proxy ~/repos/cue some/other/module
//
// Although standard library packages will work by default, the docs can take a
// while to appear the first time because the Go repo must be cloned and
// processed. If you clone the repo yourself (https://go.googlesource.com/go),
// you can provide its location with the -gorepo flag to save a little time.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/static"
	thirdparty "golang.org/x/pkgsite/third_party"
)

const defaultAddr = "localhost:8080" // default webserver address

var (
	gopathMode    = flag.Bool("gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src")
	httpAddr      = flag.String("http", defaultAddr, "HTTP service address to listen for incoming requests on")
	useCache      = flag.Bool("cache", false, "fetch from the module cache")
	cacheDir      = flag.String("cachedir", "", "module cache directory (defaults to `go env GOMODCACHE`)")
	useProxy      = flag.Bool("proxy", false, "fetch from GOPROXY if not found locally")
	goRepoPath    = flag.String("gorepo", "", "path to Go repo on local filesystem")
	useListedMods = flag.Bool("list", true, "for each path, serve all modules in build list")
)

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: %s [flags] [PATHS ...]\n", os.Args[0])
		fmt.Fprintf(out, "    where each PATHS is a single path or a comma-separated list\n")
		fmt.Fprintf(out, "    (default is current directory if neither -cache nor -proxy is provided)\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	ctx := context.Background()

	paths := collectPaths(flag.Args())
	if len(paths) == 0 && !*useCache && !*useProxy {
		paths = []string{"."}
	}

	var modCacheDir string
	if *useCache || *useListedMods {
		modCacheDir = *cacheDir
		if modCacheDir == "" {
			var err error
			modCacheDir, err = defaultCacheDir()
			if err != nil {
				die("%v", err)
			}
			if modCacheDir == "" {
				die("empty value for GOMODCACHE")
			}
		}
	}

	if *useCache || *useProxy {
		fmt.Fprintf(os.Stderr, "BYPASSING LICENSE CHECKING: MAY DISPLAY NON-REDISTRIBUTABLE INFORMATION\n")
	}

	var prox *proxy.Client
	if *useProxy {
		url := os.Getenv("GOPROXY")
		if url == "" {
			die("GOPROXY environment variable is not set")
		}
		var err error
		prox, err = proxy.New(url)
		if err != nil {
			die("connecting to proxy: %s", err)
		}
	}

	if *goRepoPath != "" {
		stdlib.SetGoRepoPath(*goRepoPath)
	}

	var cacheMods []internal.Modver
	if *useListedMods && !*useCache {
		var err error
		paths, cacheMods, err = listModsForPaths(paths, modCacheDir)
		if err != nil {
			die("listing mods (consider passing -list=false): %v", err)
		}
	}

	getters, err := buildGetters(ctx, paths, *gopathMode, modCacheDir, cacheMods, prox)
	if err != nil {
		die("%s", err)
	}
	server, err := newServer(getters, prox)
	if err != nil {
		die("%s", err)
	}
	router := http.NewServeMux()
	server.Install(router.Handle, nil, nil)
	mw := middleware.Timeout(54 * time.Second)
	log.Infof(ctx, "Listening on addr http://%s", *httpAddr)
	die("%v", http.ListenAndServe(*httpAddr, mw(router)))
}

func collectPaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		paths = append(paths, strings.Split(arg, ",")...)
	}
	return paths
}

func buildGetters(ctx context.Context, paths []string, gopathMode bool, downloadDir string, cacheMods []internal.Modver, prox *proxy.Client) ([]fetch.ModuleGetter, error) {
	getters := buildPathGetters(ctx, paths, gopathMode)
	if downloadDir != "" {
		g, err := fetch.NewFSProxyModuleGetter(downloadDir, cacheMods)
		if err != nil {
			return nil, err
		}
		getters = append(getters, g)
	}
	if prox != nil {
		getters = append(getters, fetch.NewProxyModuleGetter(prox, source.NewClient(time.Second)))
	}
	return getters, nil
}

func buildPathGetters(ctx context.Context, paths []string, gopathMode bool) []fetch.ModuleGetter {
	var getters []fetch.ModuleGetter
	loaded := len(paths)
	for _, path := range paths {
		var (
			mg  fetch.ModuleGetter
			err error
		)
		if gopathMode {
			mg, err = fetchdatasource.NewGOPATHModuleGetter(path)
		} else {
			mg, err = fetch.NewDirectoryModuleGetter("", path)
		}
		if err != nil {
			log.Error(ctx, err)
			loaded--
		} else {
			getters = append(getters, mg)
		}
	}

	if loaded == 0 && len(paths) > 0 {
		die("failed to load module(s) at %v", paths)
	}
	return getters
}

func newServer(getters []fetch.ModuleGetter, prox *proxy.Client) (*frontend.Server, error) {
	lds := fetchdatasource.Options{
		Getters:              getters,
		ProxyClientForLatest: prox,
		BypassLicenseCheck:   true,
	}.New()
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: func(context.Context) internal.DataSource { return lds },
		TemplateFS:       template.TrustedFSFromEmbed(static.FS),
		StaticFS:         static.FS,
		ThirdPartyFS:     thirdparty.FS,
	})
	if err != nil {
		return nil, err
	}
	for _, g := range getters {
		p, fsys := g.SourceFS()
		if p != "" {
			server.InstallFS(p, fsys)
		}
	}
	return server, nil
}

func defaultCacheDir() (string, error) {
	out, err := runGo("", "env", "GOMODCACHE")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// listedMod has a subset of the fields written by `go list -m`.
type listedMod struct {
	internal.Modver
	GoMod    string // absolute path to go.mod file; in download cache or replaced
	Indirect bool
}

var listModules = _listModules

func _listModules(dir string) ([]listedMod, error) {
	out, err := runGo(dir, "list", "-json", "-m", "-mod", "readonly", "all")
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(out))
	var ms []listedMod
	for d.More() {
		var m listedMod
		if err := d.Decode(&m); err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	return ms, nil
}

func runGo(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running go with %q: %v: %s", args, err, out)
	}
	return out, nil
}

func listModsForPaths(paths []string, cacheDir string) ([]string, []internal.Modver, error) {
	var outPaths []string
	var cacheMods []internal.Modver
	for _, p := range paths {
		lms, err := listModules(p)
		if err != nil {
			return nil, nil, err
		}
		for _, lm := range lms {
			// Ignore indirect modules.
			if lm.Indirect {
				continue
			}
			if lm.GoMod == "" {
				return nil, nil, errors.New("empty GoMod: please file a pkgsite bug at https://go.dev/issues/new")
			}
			if strings.HasPrefix(lm.GoMod, cacheDir) {
				cacheMods = append(cacheMods, lm.Modver)
			} else { // probably the result of a replace directive
				outPaths = append(outPaths, filepath.Dir(lm.GoMod))
			}
		}
	}
	return outPaths, cacheMods, nil
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	os.Exit(1)
}
