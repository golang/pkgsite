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
//   cd ~/repos/cue && pkgsite
//
// You can also serve docs from your module cache, directly from the proxy
// (it uses the GOPROXY environment variable), or both:
//
//   pkgsite -cache -proxy
//
// With either -cache or -proxy, pkgsite won't look for a module in the current
// directory. You can still provide modules on the local filesystem by listing
// their paths:
//
//   pkgsite -cache -proxy ~/repos/cue some/other/module
//
// Although standard library packages will work by default, the docs can take a
// while to appear the first time because the Go repo must be cloned and
// processed. If you clone the repo yourself (https://go.googlesource.com/go),
// you can provide its location with the -gorepo flag to save a little time.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	staticFlag = flag.String("static", "", "OBSOLETE - DO NOT USE")
	gopathMode = flag.Bool("gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src")
	httpAddr   = flag.String("http", defaultAddr, "HTTP service address to listen for incoming requests on")
	useCache   = flag.Bool("cache", false, "fetch from the module cache")
	cacheDir   = flag.String("cachedir", "", "module cache directory (defaults to `go env GOMODCACHE`)")
	useProxy   = flag.Bool("proxy", false, "fetch from GOPROXY if not found locally")
	goRepoPath = flag.String("gorepo", "", "path to Go repo on local filesystem")
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

	if *staticFlag != "" {
		fmt.Fprintf(os.Stderr, "-static is ignored. It is obsolete and may be removed in a future version.\n")
	}

	paths := collectPaths(flag.Args())
	if len(paths) == 0 && !*useCache && !*useProxy {
		paths = []string{"."}
	}

	var modCacheDir string
	if *useCache {
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

	server, err := newServer(ctx, paths, *gopathMode, modCacheDir, prox)
	if err != nil {
		die("%s", err)
	}
	router := http.NewServeMux()
	server.Install(router.Handle, nil, nil)
	mw := middleware.Timeout(54 * time.Second)
	log.Infof(ctx, "Listening on addr http://%s", *httpAddr)
	die("%v", http.ListenAndServe(*httpAddr, mw(router)))
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	os.Exit(1)
}

func collectPaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		paths = append(paths, strings.Split(arg, ",")...)
	}
	return paths
}

func newServer(ctx context.Context, paths []string, gopathMode bool, downloadDir string, prox *proxy.Client) (*frontend.Server, error) {
	getters := buildGetters(ctx, paths, gopathMode)
	if downloadDir != "" {
		g, err := fetch.NewFSProxyModuleGetter(downloadDir)
		if err != nil {
			return nil, err
		}
		getters = append(getters, g)
	}
	if prox != nil {
		getters = append(getters, fetch.NewProxyModuleGetter(prox, source.NewClient(time.Second)))
	}
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

func buildGetters(ctx context.Context, paths []string, gopathMode bool) []fetch.ModuleGetter {
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

func defaultCacheDir() (string, error) {
	out, err := exec.Command("go", "env", "GOMODCACHE").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running 'go env GOMODCACHE': %v: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
