// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/logging"
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

	if err := config.Init(ctx); err != nil {
		log.Fatalf("config.Init: %v", err)
	}
	config.Dump(os.Stderr)

	// Wrap the postgres driver with OpenCensus instrumentation.
	ocDriver, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Fatalf("unable to register our ocsql driver: %v\n", err)
	}
	db, err := postgres.Open(ocDriver, config.DBConnInfo())
	if err != nil {
		log.Fatalf("postgres.Open: %v", err)
	}
	defer db.Close()

	server, err := frontend.NewServer(db, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf("frontend.NewServer: %v", err)
	}

	views := append(ochttp.DefaultServerViews, dcensus.ViewByCodeRouteMethod)
	if err := dcensus.Init(views...); err != nil {
		log.Fatalf("dcensus.Init: %v", err)
	}
	// We are not currently forwarding any ports on AppEngine, so serving debug
	// information is broken.
	if !config.OnAppEngine() {
		dcensusServer, err := dcensus.NewServer(views...)
		if err != nil {
			log.Fatalf("dcensus.NewServer: %v", err)
		}
		go http.ListenAndServe(config.DebugAddr("localhost:8081"), dcensusServer)
	}

	requestLogger := getLogger(ctx)
	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.SecureHeaders(),
		middleware.Timeout(1*time.Minute),
	)

	addr := config.HostAddr("localhost:8080")
	log.Printf("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, mw(server)))
}

func getLogger(ctx context.Context) middleware.Logger {
	if config.OnAppEngine() {
		logClient, err := logging.NewClient(ctx, config.ProjectID())
		if err != nil {
			log.Fatalf("logging.NewClient: %v", err)
		}
		return logClient.Logger("frontend-log")
	}
	return middleware.LocalLogger{}
}
