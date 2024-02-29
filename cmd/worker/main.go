// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The worker command runs a service with the primary job of fetching modules
// from a proxy and writing them to the database.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"go.opencensus.io/plugin/ochttp"
	octrace "go.opencensus.io/trace"
	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	mtimeout "golang.org/x/pkgsite/internal/middleware/timeout"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue/gcpqueue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/trace"
	"golang.org/x/pkgsite/internal/worker"
)

var (
	timeout   = serverconfig.GetEnvInt(context.Background(), "GO_DISCOVERY_WORKER_TIMEOUT_MINUTES", 10)
	queueName = serverconfig.GetEnv("GO_DISCOVERY_WORKER_TASK_QUEUE", "")
	workers   = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	// flag used in call to safehtml/template.TrustedSourceFromFlag
	_                  = flag.String("static", "static", "path to folder containing static files served")
	bypassLicenseCheck = flag.Bool("bypass_license_check", false, "insert all data into the DB, even for non-redistributable paths")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	cfg.Dump(os.Stdout)

	if cfg.UseProfiler {
		if err := profiler.Start(profiler.Config{}); err != nil {
			log.Fatalf(ctx, "profiler.Start: %v", err)
		}
	}

	db, err := cmdconfig.OpenDB(ctx, cfg, *bypassLicenseCheck)
	if err != nil {
		log.Fatalf(ctx, "%v", err)
	}
	defer db.Close()

	if err := worker.PopulateExcluded(ctx, cfg, db); err != nil {
		log.Fatal(ctx, err)
	}

	indexClient, err := index.New(cfg.IndexURL)
	if err != nil {
		log.Fatal(ctx, err)
	}
	proxyClient, err := proxy.New(cfg.ProxyURL, new(ochttp.Transport))
	if err != nil {
		log.Fatal(ctx, err)
	}
	sourceClient := source.NewClient(&http.Client{
		Transport: &ochttp.Transport{},
		Timeout:   config.SourceTimeout,
	})
	expg := cmdconfig.ExperimentGetter(ctx, cfg)
	fetchQueue, err := gcpqueue.New(ctx, cfg, queueName, *workers, expg,
		func(ctx context.Context, modulePath, version string) (int, error) {
			f := &worker.Fetcher{
				ProxyClient:  proxyClient,
				SourceClient: sourceClient,
				DB:           db,
			}
			code, _, err := f.FetchAndUpdateState(ctx, modulePath, version, cfg.AppVersionLabel())
			return code, err
		})
	if err != nil {
		log.Fatalf(ctx, "gcpqueue.New: %v", err)
	}

	reporter := cmdconfig.Reporter(ctx, cfg)
	trace.SetTraceFunction(func(ctx context.Context, name string) (context.Context, trace.Span) {
		return octrace.StartSpan(ctx, name)
	})
	redisCacheClient := getCacheRedis(ctx, cfg)
	redisBetaCacheClient := getBetaCacheRedis(ctx, cfg)
	experimenter := cmdconfig.Experimenter(ctx, cfg, expg, reporter)
	server, err := worker.NewServer(cfg, worker.ServerConfig{
		DB:                   db,
		IndexClient:          indexClient,
		ProxyClient:          proxyClient,
		SourceClient:         sourceClient,
		RedisCacheClient:     redisCacheClient,
		RedisBetaCacheClient: redisBetaCacheClient,
		Queue:                fetchQueue,
		Reporter:             reporter,
		StaticPath:           template.TrustedSourceFromFlag(flag.Lookup("static").Value),
		GetExperiments:       experimenter.Experiments,
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	router := dcensus.NewRouter(nil)
	server.Install(router.Handle)

	views := append(dcensus.ServerViews,
		worker.EnqueueResponseCount,
		worker.ProcessingLag,
		worker.UnprocessedModules,
		worker.UnprocessedNewModules,
		worker.DBProcesses,
		worker.DBWaitingProcesses,
		worker.SheddedFetchCount,
		worker.FetchLatencyDistribution,
		worker.FetchResponseCount,
		worker.FetchPackageCount)
	if err := dcensus.Init(cfg, views...); err != nil {
		log.Fatal(ctx, err)
	}

	iap := middleware.Identity()
	if aud := os.Getenv("GO_DISCOVERY_IAP_AUDIENCE"); aud != "" {
		iap = middleware.ValidateIAPHeader(aud)
	}

	mw := middleware.Chain(
		middleware.RequestInfo(), // must be first
		middleware.RequestLog(cmdconfig.Logger(ctx, cfg, "worker-log")),
		mtimeout.Timeout(time.Duration(timeout)*time.Minute),
		iap,
		middleware.Experiment(experimenter),
	)
	http.Handle("/", mw(router))

	dh, err := server.DebugHandler()
	if err != nil {
		log.Fatal(ctx, err)
	}
	http.Handle("/debug/", mw(http.StripPrefix("/debug", dh)))

	addr := cfg.HostAddr("localhost:8000")
	log.Infof(ctx, "Timeout is %d minutes", timeout)
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}

func getCacheRedis(ctx context.Context, cfg *config.Config) *redis.Client {
	return getRedis(ctx, cfg.RedisCacheHost, cfg.RedisCachePort, 0, 6*time.Second)
}

func getBetaCacheRedis(ctx context.Context, cfg *config.Config) *redis.Client {
	return getRedis(ctx, cfg.RedisBetaCacheHost, cfg.RedisCachePort, 0, 6*time.Second)
}

func getRedis(ctx context.Context, host, port string, writeTimeout, readTimeout time.Duration) *redis.Client {
	if host == "" {
		return nil
	}
	var dialTimeout time.Duration
	if dl, ok := ctx.Deadline(); ok {
		dialTimeout = time.Until(dl)
	}
	return redis.NewClient(&redis.Options{
		Addr:         host + ":" + port,
		DialTimeout:  dialTimeout,
		WriteTimeout: writeTimeout,
		ReadTimeout:  readTimeout,
	})
}
