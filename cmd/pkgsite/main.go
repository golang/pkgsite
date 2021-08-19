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
	"net/http"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/localdatasource"
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
	flag.Parse()
	ctx := context.Background()

	paths := flag.Arg(0)
	if paths == "" {
		paths = "."
	}

	lds := localdatasource.New(source.NewClient(time.Second))
	dsg := func(context.Context) internal.DataSource { return lds }
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: dsg,
		StaticPath:       template.TrustedSourceFromFlag(flag.Lookup("static").Value),
	})
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}

	load(ctx, lds, paths)

	router := dcensus.NewRouter(frontend.TagRoute)
	server.Install(router.Handle, nil, nil)

	mw := middleware.Timeout(54 * time.Second)
	log.Infof(ctx, "Listening on addr %s", *httpAddr)
	log.Fatal(ctx, http.ListenAndServe(*httpAddr, mw(router)))
}

// load loads local modules from pathList.
func load(ctx context.Context, ds *localdatasource.DataSource, pathList string) {
	paths := strings.Split(pathList, ",")
	loaded := len(paths)
	for _, path := range paths {
		var (
			mg  fetch.ModuleGetter
			err error
		)
		if *gopathMode {
			mg, err = localdatasource.NewGOPATHModuleGetter(path)
		} else {
			mg, err = fetch.NewDirectoryModuleGetter("", path)
		}
		if err != nil {
			log.Error(ctx, err)
			loaded--
		} else {
			ds.AddModuleGetter(mg)
		}
	}

	if loaded == 0 {
		log.Fatalf(ctx, "failed to load module(s) at %s", pathList)
	}
}
