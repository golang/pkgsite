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
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/proxydatasource"
)

var (
	staticPath      = flag.String("static", "content/static", "path to folder containing static files served")
	reloadTemplates = flag.Bool("reload_templates", false, "reload templates on each page load (to be used during development)")
	directProxy     = flag.String("direct_proxy", "", "if set to a valid URL, uses the module proxy referred to by this URL "+
		"as a direct backend, bypassing the database")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	if err := config.Init(ctx); err != nil {
		log.Fatal(err)
	}
	config.Dump(os.Stderr)
	var ds frontend.DataSource
	if *directProxy != "" {
		proxyClient, err := proxy.New(*directProxy)
		if err != nil {
			log.Fatal(err)
		}
		ds = proxydatasource.New(proxyClient)
	} else {
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
		ds = db
	}

	server, err := frontend.NewServer(ds, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf("frontend.NewServer: %v", err)
	}
	router := dcensus.NewRouter()
	server.Install(router.Handle)

	views := append(ochttp.DefaultServerViews, dcensus.ViewByCodeRouteMethod)
	if err := dcensus.Init(views...); err != nil {
		log.Fatal(err)
	}
	// We are not currently forwarding any ports on AppEngine, so serving debug
	// information is broken.
	if !config.OnAppEngine() {
		dcensusServer, err := dcensus.NewServer(views...)
		if err != nil {
			log.Fatal(err)
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
	log.Fatal(http.ListenAndServe(addr, mw(router)))
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
