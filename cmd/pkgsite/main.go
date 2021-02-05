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
//  pkgsite [flag]
//
// The flags are:
//
//  -local=path1,path2
//      Accepts a GOPATH-like collection of local paths for modules to load to memory
//  -gopath_mode=false
//      Assume that local modules' paths are relative to GOPATH/src
package main

import (
	"context"
	"flag"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/localdatasource"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
)

var (
	_          = flag.String("static", "content/static", "path to folder containing static files served")
	localPaths = flag.String("local", "", "run locally, accepts a GOPATH-like collection of local paths for modules to load to memory")
	gopathMode = flag.Bool("gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src, used only with -local")
)

func main() {
	flag.Parse()
	ctx := context.Background()
	var dsg func(context.Context) internal.DataSource
	if *localPaths == "" {
		log.Fatalf(ctx, "-local is not set")
	}

	lds := localdatasource.New()
	dsg = func(context.Context) internal.DataSource { return lds }
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter: dsg,
		StaticPath:       template.TrustedSourceFromFlag(flag.Lookup("static").Value),
	})
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}
	lds, ok := dsg(ctx).(*localdatasource.DataSource)
	if ok {
		load(ctx, lds, *localPaths)
	}

	router := dcensus.NewRouter(frontend.TagRoute)
	server.Install(router.Handle, nil, nil)

	mw := middleware.Chain(
		middleware.RedirectedFrom(),
		middleware.LatestVersions(server.GetLatestInfo), // must come before caching for version badge to work
		middleware.Timeout(54*time.Second),
	)
	addr := "localhost:6060"
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, mw(router)))
}

// load loads local modules from pathList.
func load(ctx context.Context, ds *localdatasource.DataSource, pathList string) {
	paths := filepath.SplitList(pathList)
	loaded := len(paths)
	for _, path := range paths {
		var err error
		if *gopathMode {
			err = ds.LoadFromGOPATH(ctx, path)
		} else {
			err = ds.Load(ctx, path)
		}
		if err != nil {
			log.Error(ctx, err)
			loaded--
		}
	}

	if loaded == 0 {
		log.Fatalf(ctx, "failed to load module(s) at %s", pathList)
	}
}
