// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/logging"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/dcensus"
	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"

	"contrib.go.opencensus.io/integrations/ocsql"
)

var (
	timeout    = config.GetEnv("GO_DISCOVERY_ETL_TIMEOUT_MINUTES", "10")
	queueName  = config.GetEnv("GO_DISCOVERY_ETL_TASK_QUEUE", "dev-fetch-tasks")
	workers    = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	staticPath = flag.String("static", "content/static", "path to folder containing static files served")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	if err := config.Init(ctx); err != nil {
		log.Fatal(err)
	}
	config.Dump(os.Stderr)

	// Wrap the postgres driver with OpenCensus instrumentation.
	driverName, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Fatalf("unable to register the ocsql driver: %v\n", err)
	}
	db, err := postgres.Open(driverName, config.DBConnInfo())
	if err != nil {
		log.Fatalf("postgres.Open: %v", err)
	}
	defer db.Close()

	indexClient, err := index.New(config.IndexURL())
	if err != nil {
		log.Fatal(err)
	}
	proxyClient, err := proxy.New(config.ProxyURL())
	if err != nil {
		log.Fatal(err)
	}

	var q etl.Queue
	if config.OnAppEngine() {
		client, err := cloudtasks.NewClient(ctx)
		if err != nil {
			log.Fatal(err)
		}
		q = etl.NewGCPQueue(client, queueName)
	} else {
		q = etl.NewInMemoryQueue(ctx, proxyClient, db, *workers)
	}

	server, err := etl.NewServer(db, indexClient, proxyClient, q, *staticPath)
	if err != nil {
		log.Fatal(err)
	}
	router := dcensus.NewRouter(nil)
	server.Install(router.Handle)

	views := append(dcensus.ClientViews, dcensus.ServerViews...)
	if err := dcensus.Init(views...); err != nil {
		log.Fatal(err)
	}
	// We are not currently forwarding any ports on AppEngine, so serving debug
	// information is broken.
	if !config.OnAppEngine() {
		dcensusServer, err := dcensus.NewServer()
		if err != nil {
			log.Fatal(err)
		}
		go http.ListenAndServe(config.DebugAddr("localhost:8001"), dcensusServer)
	}

	handlerTimeout, err := strconv.Atoi(timeout)
	if err != nil {
		log.Fatalf("strconv.Atoi(%q): %v", timeout, err)
	}
	requestLogger := getLogger(ctx)
	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.Timeout(time.Duration(handlerTimeout)*time.Minute),
	)
	http.Handle("/", mw(router))

	addr := config.HostAddr("localhost:8000")
	log.Printf("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func getLogger(ctx context.Context) middleware.Logger {
	if config.OnAppEngine() {
		logClient, err := logging.NewClient(ctx, config.ProjectID())
		if err != nil {
			log.Fatalf("logging.NewClient: %v", err)
		}
		return logClient.Logger("etl-log")
	}
	return middleware.LocalLogger{}
}
