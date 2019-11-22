// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"bufio"
	"context"
	"flag"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/errorreporting"
	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/dcensus"
	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/index"

	"golang.org/x/discovery/internal/log"
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

	readProxyRemoved()

	// Wrap the postgres driver with OpenCensus instrumentation.
	driverName, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Fatalf("unable to register the ocsql driver: %v\n", err)
	}
	ddb, err := database.Open(driverName, config.DBConnInfo())
	if err != nil {
		log.Fatalf("database.Open: %v", err)
	}
	db := postgres.New(ddb)
	defer db.Close()

	indexClient, err := index.New(config.IndexURL())
	if err != nil {
		log.Fatal(err)
	}
	proxyClient, err := proxy.New(config.ProxyURL())
	if err != nil {
		log.Fatal(err)
	}
	fetchQueue := queue(ctx, proxyClient, db)
	reportingClient := reportingClient(ctx)
	redisClient := getRedis(ctx)
	server, err := etl.NewServer(db, indexClient, proxyClient, redisClient, fetchQueue, reportingClient, *staticPath)
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
	requestLogger := logger(ctx)
	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.Timeout(time.Duration(handlerTimeout)*time.Minute),
	)
	http.Handle("/", mw(router))

	addr := config.HostAddr("localhost:8000")
	log.Infof("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func queue(ctx context.Context, proxyClient *proxy.Client, db *postgres.DB) etl.Queue {
	if !config.OnAppEngine() {
		return etl.NewInMemoryQueue(ctx, proxyClient, db, *workers)
	}
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return etl.NewGCPQueue(client, queueName)
}

func getRedis(ctx context.Context) *redis.Client {
	if config.RedisHAHost() != "" {
		return redis.NewClient(&redis.Options{
			Addr: config.RedisHAHost() + ":" + config.RedisHAPort(),
			// We update completions with one big pipeline, so we need long write
			// timeouts. ReadTimeout is increased only to be consistent with
			// WriteTimeout.
			WriteTimeout: 5 * time.Minute,
			ReadTimeout:  5 * time.Minute,
		})
	}
	return nil
}

func reportingClient(ctx context.Context) *errorreporting.Client {
	if !config.OnAppEngine() {
		return nil
	}
	reporter, err := errorreporting.NewClient(ctx, config.ProjectID(), errorreporting.Config{
		ServiceName: config.ServiceID(),
		OnError: func(err error) {
			log.Errorf("Error reporting failed: %v", err)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return reporter
}

func logger(ctx context.Context) middleware.Logger {
	if config.OnAppEngine() {
		logger, err := log.UseStackdriver(ctx, "etl-log")
		if err != nil {
			log.Fatal(err)
		}
		return logger
	}
	return middleware.LocalLogger{}
}

// Read a file of module versions that we should ignore because
// the are in the index but not stored in the proxy.
// Format of the file: each line is
//     module@version
func readProxyRemoved() {
	filename := config.GetEnv("GO_DISCOVERY_PROXY_REMOVED", "")
	if filename == "" {
		return
	}
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		etl.ProxyRemoved[strings.TrimSpace(scan.Text())] = true
	}
	if err := scan.Err(); err != nil {
		log.Fatalf("scanning %s: %v", filename, err)
	}
	log.Infof("read %d excluded module versions from %s", len(etl.ProxyRemoved), filename)
}
