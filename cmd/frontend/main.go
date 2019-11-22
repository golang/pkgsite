// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/dcensus"
	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/log"
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
			log.Fatalf("unable to register the ocsql driver: %v\n", err)
		}
		ddb, err := database.Open(ocDriver, config.DBConnInfo())
		if err != nil {
			log.Fatalf("database.Open: %v", err)
		}
		db := postgres.New(ddb)
		defer db.Close()
		ds = db
	}
	var haClient *redis.Client
	if config.RedisHAHost() != "" {
		haClient = redis.NewClient(&redis.Options{
			Addr: config.RedisHAHost() + ":" + config.RedisHAPort(),
		})
	}
	server, err := frontend.NewServer(ds, haClient, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf("frontend.NewServer: %v", err)
	}
	router := dcensus.NewRouter(frontend.TagRoute)
	var cacheClient *redis.Client
	if config.RedisHost() != "" {
		cacheClient = redis.NewClient(&redis.Options{
			Addr: config.RedisHost() + ":" + config.RedisPort(),
		})
	}
	server.Install(router.Handle, cacheClient)

	views := append(dcensus.ServerViews,
		postgres.SearchLatencyDistribution,
		postgres.SearchResponseCount,
		middleware.CacheResultCount,
		middleware.CacheErrorCount,
		middleware.QuotaResultCount,
	)
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
		go http.ListenAndServe(config.DebugAddr("localhost:8081"), dcensusServer)
	}

	panicHandler, err := server.PanicHandler()
	if err != nil {
		log.Fatal(err)
	}
	requestLogger := getLogger(ctx)
	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.Quota(config.Quota()),
		middleware.SecureHeaders(),                     // must come before any caching for nonces to work
		middleware.LatestVersion(server.LatestVersion), // must come before caching for version badge to work
		middleware.Panic(panicHandler),
		middleware.Timeout(1*time.Minute),
	)

	addr := config.HostAddr("localhost:8080")
	log.Infof("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, mw(router)))
}

func getLogger(ctx context.Context) middleware.Logger {
	if config.OnAppEngine() {
		logger, err := log.UseStackdriver(ctx, "frontend-log")
		if err != nil {
			log.Fatal(err)
		}
		return logger
	}
	return middleware.LocalLogger{}
}
