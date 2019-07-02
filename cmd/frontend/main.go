// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"contrib.go.opencensus.io/integrations/ocsql"
	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/dcensus"
	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

var (
	staticPath      = flag.String("static", "content/static", "path to folder containing static files served")
	reloadTemplates = flag.Bool("reload_templates", false, "reload templates on each page load (to be used during development)")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	dbinfo, err := config.DBConnInfo(ctx)
	if err != nil {
		log.Fatalf("Unable to construct database connection info string: %v", err)
	}
	// Wrap the postgres driver with OpenCensus instrumentation.
	ocDriver, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Fatalf("unable to register our ocsql driver: %v\n", err)
	}
	db, err := postgres.Open(ocDriver, dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open: %v", err)
	}
	defer db.Close()

	server, err := frontend.NewServer(db, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf("frontend.NewServer: %v", err)
	}

	views := append(ochttp.DefaultServerViews, dcensus.ViewByCodeRouteMethod)
	dcensusServer, err := dcensus.NewServer(views...)
	if err != nil {
		log.Fatalf("dcensus.NewServer: %v", err)
	}
	go http.ListenAndServe(config.DebugAddr("localhost:8081"), dcensusServer)

	addr := config.HostAddr("localhost:8080")
	log.Printf("Listening on addr %s", addr)

	mw := middleware.Chain(
		middleware.SecureHeaders(),
		middleware.Timeout(1*time.Minute),
	)
	log.Fatal(http.ListenAndServe(addr, mw(server)))
}
