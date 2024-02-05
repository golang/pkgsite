// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Pkgsite extracts and generates documentation for Go programs.
// It runs as a web server and presents the documentation as a
// web page.
//
// To install, run `go install golang.org/x/pkgsite/cmd/pkgsite@latest`.
//
// With no arguments, pkgsite will serve docs for main modules relative to the
// current directory, i.e. the modules listed by `go list -m`. This is
// typically the module defined by the nearest go.mod file in a parent
// directory. However, this may include multiple main modules when using a
// go.work file to define a [workspace].
//
// For example, both of the following forms could be used to work
// on the module defined in repos/cue/go.mod:
//
// The single module form:
//
//	cd repos/cue && pkgsite
//
// The multiple module form:
//
//	go work init repos/cue repos/other && pkgsite
//
// By default, the resulting server will also serve all of the module's
// dependencies at their required versions. You can disable serving the
// required modules by passing -list=false.
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
//
// [workspace]: https://go.dev/ref/mod#workspaces
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/pkgsite/cmd/internal/pkgsite"
	"golang.org/x/pkgsite/internal/browser"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware/timeout"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/stdlib"
)

const defaultAddr = "localhost:8080" // default webserver address

var (
	httpAddr   = flag.String("http", defaultAddr, "HTTP service address to listen for incoming requests on")
	goRepoPath = flag.String("gorepo", "", "path to Go repo on local filesystem")
	useProxy   = flag.Bool("proxy", false, "fetch from GOPROXY if not found locally")
	openFlag   = flag.Bool("open", false, "open a browser window to the server's address")
	// other flags are bound to ServerConfig below
)

func main() {
	var serverCfg pkgsite.ServerConfig

	flag.BoolVar(&serverCfg.GOPATHMode, "gopath_mode", false, "assume that local modules' Paths are relative to GOPATH/src")
	flag.BoolVar(&serverCfg.UseCache, "cache", false, "fetch from the module cache")
	flag.StringVar(&serverCfg.CacheDir, "cachedir", "", "module cache directory (defaults to `go env GOMODCACHE`)")
	flag.BoolVar(&serverCfg.UseListedMods, "list", true, "for each path, serve all modules in build list")
	flag.BoolVar(&serverCfg.DevMode, "dev", false, "enable developer mode (reload templates on each page load, serve non-minified JS/CSS, etc.)")
	flag.StringVar(&serverCfg.DevModeStaticDir, "static", "static", "path to folder containing static files served")
	serverCfg.UseLocalStdlib = true
	serverCfg.GoRepoPath = *goRepoPath

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: %s [flags] [PATHS ...]\n", os.Args[0])
		fmt.Fprintf(out, "    where each PATHS is a single path or a comma-separated list\n")
		fmt.Fprintf(out, "    (default is current directory if neither -cache nor -proxy is provided)\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	serverCfg.Paths = collectPaths(flag.Args())

	if serverCfg.UseCache || *useProxy {
		fmt.Fprintf(os.Stderr, "BYPASSING LICENSE CHECKING: MAY DISPLAY NON-REDISTRIBUTABLE INFORMATION\n")
	}

	if *useProxy {
		url := os.Getenv("GOPROXY")
		if url == "" {
			die("GOPROXY environment variable is not set")
		}
		var err error
		serverCfg.Proxy, err = proxy.New(url, nil)
		if err != nil {
			die("connecting to proxy: %s", err)
		}
	}

	if *goRepoPath != "" {
		stdlib.SetGoRepoPath(*goRepoPath)
	}

	ctx := context.Background()
	server, err := pkgsite.BuildServer(ctx, serverCfg)
	if err != nil {
		die(err.Error())
	}

	addr := *httpAddr
	if addr == "" {
		addr = ":http"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		die(err.Error())
	}

	url := "http://" + addr
	log.Infof(ctx, "Listening on addr %s", url)

	if *openFlag {
		go func() {
			if !browser.Open(url) {
				log.Infof(ctx, "Failed to open browser window. Please visit %s in your browser.", url)
			}
		}()
	}

	router := http.NewServeMux()
	server.Install(router.Handle, nil, nil)
	mw := timeout.Timeout(54 * time.Second)
	srv := &http.Server{Addr: addr, Handler: mw(router)}
	die("%v", srv.Serve(ln))
}

func die(format string, args ...any) {
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
