// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

	"cloud.google.com/go/profiler"
	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal"
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

	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	cfg.Dump(os.Stderr)

	if cfg.UseProfiler {
		if err := profiler.Start(profiler.Config{}); err != nil {
			log.Fatalf(ctx, "profiler.Start: %v", err)
		}
	}

	var (
		ds  internal.DataSource
		exp internal.ExperimentSource
	)
	if *directProxy != "" {
		proxyClient, err := proxy.New(*directProxy)
		if err != nil {
			log.Fatal(ctx, err)
		}
		ds = proxydatasource.New(proxyClient)
		exp = internal.NewLocalExperimentSource(readLocalExperiments(ctx))
	} else {
		// Wrap the postgres driver with OpenCensus instrumentation.
		ocDriver, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
		if err != nil {
			log.Fatalf(ctx, "unable to register the ocsql driver: %v\n", err)
		}
		ddb, err := database.Open(ocDriver, cfg.DBConnInfo())
		if err != nil {
			log.Fatalf(ctx, "database.Open: %v", err)
		}
		db := postgres.New(ddb)
		defer db.Close()
		ds = db
		exp = db
	}
	var haClient *redis.Client
	if cfg.RedisHAHost != "" {
		haClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisHAHost + ":" + cfg.RedisHAPort,
		})
	}
	server, err := frontend.NewServer(ds, haClient, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}
	router := dcensus.NewRouter(frontend.TagRoute)
	var cacheClient *redis.Client
	if cfg.RedisCacheHost != "" {
		cacheClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisCacheHost + ":" + cfg.RedisCachePort,
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
	if err := dcensus.Init(cfg, views...); err != nil {
		log.Fatal(ctx, err)
	}
	// We are not currently forwarding any ports on AppEngine, so serving debug
	// information is broken.
	if !cfg.OnAppEngine() {
		dcensusServer, err := dcensus.NewServer()
		if err != nil {
			log.Fatal(ctx, err)
		}
		go http.ListenAndServe(cfg.DebugAddr("localhost:8081"), dcensusServer)
	}

	panicHandler, err := server.PanicHandler()
	if err != nil {
		log.Fatal(ctx, err)
	}
	requestLogger := getLogger(ctx, cfg)
	experimenter, err := middleware.NewExperimenter(ctx, 1*time.Minute, exp, requestLogger)
	if err != nil {
		log.Fatal(ctx, err)
	}

	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.Quota(cfg.Quota),
		middleware.SecureHeaders(),                     // must come before any caching for nonces to work
		middleware.LatestVersion(server.LatestVersion), // must come before caching for version badge to work
		middleware.Panic(panicHandler),
		middleware.Timeout(54*time.Second),
		middleware.Experiment(experimenter),
	)

	addr := cfg.HostAddr("localhost:8080")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, mw(router)))
}

func getLogger(ctx context.Context, cfg *config.Config) middleware.Logger {
	if cfg.OnAppEngine() {
		logger, err := log.UseStackdriver(ctx, cfg, "frontend-log")
		if err != nil {
			log.Fatal(ctx, err)
		}
		return logger
	}
	return middleware.LocalLogger{}
}

// Read a file of experiments used to initialize the local experiment source
// for use in direct proxy mode.
// Format of the file: each line is
//     name,rollout
// For each experiment.
func readLocalExperiments(ctx context.Context) []*internal.Experiment {
	filename := config.GetEnv("GO_DISCOVERY_LOCAL_EXPERIMENTS", "")
	if filename == "" {
		return nil
	}
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(ctx, err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	var experiments []*internal.Experiment
	log.Infof(ctx, "reading experiments from %q for local development", filename)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 2 {
			log.Fatalf(ctx, "invalid experiment in file: %q", line)
		}
		name := parts[0]
		if name == "" {
			log.Fatalf(ctx, "invalid experiment in file (name cannot be empty): %q", line)
		}
		rollout, err := strconv.ParseUint(parts[1], 10, 0)
		if err != nil {
			log.Fatalf(ctx, "invalid experiment in file (invalid rollout): %v", err)
		}
		if rollout > 100 {
			log.Fatalf(ctx, "invalid experiment in file (rollout must be between 0 - 100): %q", line)
		}
		experiments = append(experiments, &internal.Experiment{
			Name:    name,
			Rollout: uint(rollout),
		})
		log.Infof(ctx, "experiment %q: rollout = %d", name, rollout)
	}
	if err := scan.Err(); err != nil {
		log.Fatalf(ctx, "scanning %s: %v", filename, err)
	}
	log.Infof(ctx, "found %d experiment(s)", len(experiments))
	return experiments
}
