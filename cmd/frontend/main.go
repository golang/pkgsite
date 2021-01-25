// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The frontend runs a service to serve user-facing traffic.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/localdatasource"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/proxydatasource"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
)

var (
	queueName      = config.GetEnv("GO_DISCOVERY_FRONTEND_TASK_QUEUE", "")
	workers        = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	_              = flag.String("static", "content/static", "path to folder containing static files served")
	thirdPartyPath = flag.String("third_party", "third_party", "path to folder containing third-party libraries")
	devMode        = flag.Bool("dev", false, "enable developer mode (reload templates on each page load, serve non-minified JS/CSS, etc.)")
	disableCSP     = flag.Bool("nocsp", false, "disable Content Security Policy")
	proxyURL       = flag.String("proxy_url", "https://proxy.golang.org", "Uses the module proxy referred to by this URL "+
		"for direct proxy mode and frontend fetches")
	directProxy = flag.Bool("direct_proxy", false, "if set to true, uses the module proxy referred to by this URL "+
		"as a direct backend, bypassing the database")
	localPaths         = flag.String("local", "", "run locally, accepts a GOPATH-like collection of local paths for modules to load to memory")
	gopathMode         = flag.Bool("gopath_mode", false, "assume that local modules' paths are relative to GOPATH/src, used only with -local")
	bypassLicenseCheck = flag.Bool("bypass_license_check", false, "display all information, even for non-redistributable paths")
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

	log.SetLevel(cfg.LogLevel)

	var (
		dsg        func(context.Context) internal.DataSource
		fetchQueue queue.Queue
	)
	if *bypassLicenseCheck {
		log.Info(ctx, "BYPASSING LICENSE CHECKING: DISPLAYING NON-REDISTRIBUTABLE INFORMATION")
	}

	log.Infof(ctx, "cmd/frontend: initializing cmdconfig.ExperimentGetter")
	expg := cmdconfig.ExperimentGetter(ctx, cfg)
	log.Infof(ctx, "cmd/frontend: initialized cmdconfig.ExperimentGetter")

	if *localPaths != "" {
		lds := localdatasource.New()
		dsg = func(context.Context) internal.DataSource { return lds }
	} else {
		proxyClient, err := proxy.New(*proxyURL)
		if err != nil {
			log.Fatal(ctx, err)
		}

		if *directProxy {
			var pds *proxydatasource.DataSource
			if *bypassLicenseCheck {
				pds = proxydatasource.NewBypassingLicenseCheck(proxyClient)
			} else {
				pds = proxydatasource.New(proxyClient)
			}
			dsg = func(context.Context) internal.DataSource { return pds }
		} else {
			db, err := cmdconfig.OpenDB(ctx, cfg, *bypassLicenseCheck)
			if err != nil {
				log.Fatalf(ctx, "%v", err)
			}
			defer db.Close()
			dsg = func(context.Context) internal.DataSource { return db }
			sourceClient := source.NewClient(config.SourceTimeout)
			// The closure passed to queue.New is only used for testing and local
			// execution, not in production. So it's okay that it doesn't use a
			// per-request connection.
			fetchQueue, err = queue.New(ctx, cfg, queueName, *workers, expg,
				func(ctx context.Context, modulePath, version string) (int, error) {
					return frontend.FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, db)
				})
			if err != nil {
				log.Fatalf(ctx, "queue.New: %v", err)
			}
		}
	}

	var haClient *redis.Client
	if cfg.RedisHAHost != "" {
		haClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisHAHost + ":" + cfg.RedisHAPort,
		})
	}
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter:     dsg,
		Queue:                fetchQueue,
		CompletionClient:     haClient,
		TaskIDChangeInterval: config.TaskIDChangeIntervalFrontend,
		StaticPath:           template.TrustedSourceFromFlag(flag.Lookup("static").Value),
		ThirdPartyPath:       *thirdPartyPath,
		DevMode:              *devMode,
		AppVersionLabel:      cfg.AppVersionLabel(),
		GoogleTagManagerID:   cfg.GoogleTagManagerID,
		ServeStats:           cfg.ServeStats,
	})
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}

	if *localPaths != "" {
		lds, ok := dsg(ctx).(*localdatasource.DataSource)
		if ok {
			load(ctx, lds, *localPaths)
		}
	}

	router := dcensus.NewRouter(frontend.TagRoute)
	var cacheClient *redis.Client
	if cfg.RedisCacheHost != "" {
		cacheClient = redis.NewClient(&redis.Options{
			Addr: cfg.RedisCacheHost + ":" + cfg.RedisCachePort,
		})
	}
	server.Install(router.Handle, cacheClient, cfg.AuthValues)
	views := append(dcensus.ServerViews,
		postgres.SearchLatencyDistribution,
		postgres.SearchResponseCount,
		frontend.FetchLatencyDistribution,
		frontend.FetchResponseCount,
		frontend.PlaygroundShareRequestCount,
		frontend.VersionTypeCount,
		middleware.CacheResultCount,
		middleware.CacheErrorCount,
		middleware.CacheLatency,
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
	rc := cmdconfig.ReportingClient(ctx, cfg)
	log.Infof(ctx, "cmd/frontend: initializing cmdconfig.Experimenter")
	experimenter := cmdconfig.Experimenter(ctx, cfg, expg, rc)
	log.Infof(ctx, "cmd/frontend: initialized cmdconfig.Experimenter")

	ermw := middleware.Identity()
	if rc != nil {
		ermw = middleware.ErrorReporting(rc.Report)
	}
	mw := middleware.Chain(
		middleware.RequestLog(cmdconfig.Logger(ctx, cfg, "frontend-log")),
		middleware.AcceptRequests(http.MethodGet, http.MethodPost, http.MethodHead), // accept only GETs, POSTs and HEADs
		middleware.Quota(cfg.Quota, cacheClient),
		middleware.RedirectedFrom(),
		middleware.SecureHeaders(!*disableCSP), // must come before any caching for nonces to work
		middleware.Experiment(experimenter),
		middleware.LatestVersions(server.GetLatestInfo), // must come before caching for version badge to work
		middleware.Panic(panicHandler),
		ermw,
		middleware.Timeout(54*time.Second),
	)
	addr := cfg.HostAddr("localhost:8080")
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
