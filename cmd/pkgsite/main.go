// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is a work in progress.
//
// Pkgsite extracts and generates documentation for Go programs.
// It runs as a web server and presents the documentation as a
// web page.
// Usage:
//
//  pkgsite [flag] # Load module from current directory.
//  pkgsite [flag] [path1,path2] # Load modules from paths to memory.
//
// The flags are:
//
//  -gopath_mode=false
//      Assume that local modules' paths are relative to GOPATH/src
//  -http=:8080
//      HTTP service address to listen for incoming requests on
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/source"
)

const defaultAddr = "localhost:8080" // default webserver address

var (
	_          = flag.String("static", "static", "path to folder containing static files served")
	gopathMode = flag.Bool("gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src")
	httpAddr   = flag.String("http", defaultAddr, "HTTP service address to listen for incoming requests on")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] [PATHS ...]\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "    where PATHS is a single path or a comma-separated list\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	ctx := context.Background()

	paths := collectPaths(flag.Args())
	if len(paths) == 0 {
		paths = []string{"."}
	}

	server, err := newServer(ctx, paths, *gopathMode)
	if err != nil {
		log.Fatalf(ctx, "newServer: %v", err)
	}
	router := dcensus.NewRouter(frontend.TagRoute)
	server.Install(router.Handle, nil, nil)
	mw := middleware.Timeout(54 * time.Second)
	log.Infof(ctx, "Listening on addr %s", *httpAddr)
	log.Fatal(ctx, http.ListenAndServe(*httpAddr, mw(router)))
}

func collectPaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		paths = append(paths, strings.Split(arg, ",")...)
	}
	return paths
}

func newServer(ctx context.Context, paths []string, gopathMode bool) (*frontend.Server, error) {
	getters := buildGetters(ctx, paths, gopathMode)
	lds := fetchdatasource.Options{
		Getters:            getters,
		SourceClient:       source.NewClient(time.Second),
		BypassLicenseCheck: true,
	}.New()
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: func(context.Context) internal.DataSource { return lds },
		StaticPath:       template.TrustedSourceFromFlag(flag.Lookup("static").Value),
	})
	if err != nil {
		return nil, err
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

	if loaded == 0 {
		log.Fatalf(ctx, "failed to load module(s) at %v", paths)
	}
	return getters
}
