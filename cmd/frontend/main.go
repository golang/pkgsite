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
	"time"

	"cloud.google.com/go/profiler"
	"github.com/go-redis/redis/v8"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	vulnc "golang.org/x/vuln/client"
)

var (
	queueName      = config.GetEnv("GO_DISCOVERY_FRONTEND_TASK_QUEUE", "")
	workers        = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	staticFlag     = flag.String("static", "static", "path to folder containing static files served")
	thirdPartyPath = flag.String("third_party", "third_party", "path to folder containing third-party libraries")
	devMode        = flag.Bool("dev", false, "enable developer mode (reload templates on each page load, serve non-minified JS/CSS, etc.)")
	disableCSP     = flag.Bool("nocsp", false, "disable Content Security Policy")
	proxyURL       = flag.String("proxy_url", "https://proxy.golang.org", "Uses the module proxy referred to by this URL "+
		"for direct proxy mode and frontend fetches")
	directProxy = flag.Bool("direct_proxy", false, "if set to true, uses the module proxy referred to by this URL "+
		"as a direct backend, bypassing the database")
	bypassLicenseCheck = flag.Bool("bypass_license_check", false, "display all information, even for non-redistributable paths")
	hostAddr           = flag.String("host", "localhost:8080", "Host address for the server")
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
		dsg        func(context.Context) internal.DataSource
		fetchQueue queue.Queue
	)
	if *bypassLicenseCheck {
		log.Info(ctx, "BYPASSING LICENSE CHECKING: DISPLAYING NON-REDISTRIBUTABLE INFORMATION")
	}

	log.Infof(ctx, "cmd/frontend: initializing cmdconfig.ExperimentGetter")
	expg := cmdconfig.ExperimentGetter(ctx, cfg)
	log.Infof(ctx, "cmd/frontend: initialized cmdconfig.ExperimentGetter")

	proxyClient, err := proxy.New(*proxyURL)
	if err != nil {
		log.Fatal(ctx, err)
	}

	if *directProxy {
		ds := fetchdatasource.Options{
			Getters:              []fetch.ModuleGetter{fetch.NewProxyModuleGetter(proxyClient, source.NewClient(1*time.Minute))},
			ProxyClientForLatest: proxyClient,
			BypassLicenseCheck:   *bypassLicenseCheck,
		}.New()
		dsg = func(context.Context) internal.DataSource { return ds }
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

	rc := cmdconfig.ReportingClient(ctx, cfg)
	vc, err := vulnc.NewClient([]string{cfg.VulnDB}, vulnc.Options{
		HTTPCache: newVulndbCache(),
	})
	if err != nil {
		log.Fatalf(ctx, "vulndbc.NewClient: %v", err)
	}
	staticSource := template.TrustedSourceFromFlag(flag.Lookup("static").Value)
	server, err := frontend.NewServer(frontend.ServerConfig{
		DataSourceGetter:     dsg,
		Queue:                fetchQueue,
		TaskIDChangeInterval: config.TaskIDChangeIntervalFrontend,
		TemplateFS:           template.TrustedFSFromTrustedSource(staticSource),
		StaticFS:             os.DirFS(*staticFlag),
		StaticPath:           *staticFlag,
		ThirdPartyFS:         os.DirFS(*thirdPartyPath),
		DevMode:              *devMode,
		AppVersionLabel:      cfg.AppVersionLabel(),
		GoogleTagManagerID:   cfg.GoogleTagManagerID,
		ServeStats:           cfg.ServeStats,
		ReportingClient:      rc,
		VulndbClient:         vc,
	})
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}

	router := dcensus.NewRouter(frontend.TagRoute)
	var cacheClient *redis.Client
	if cfg.RedisCacheHost != "" {
		addr := cfg.RedisCacheHost + ":" + cfg.RedisCachePort
		cacheClient = redis.NewClient(&redis.Options{Addr: addr})
		if err := cacheClient.Ping(ctx).Err(); err != nil {
			log.Errorf(ctx, "redis at %s: %v", addr, err)
		} else {
			log.Infof(ctx, "connected to redis at %s", addr)
		}
	}
	server.Install(router.Handle, cacheClient, cfg.AuthValues)
	views := append(dcensus.ServerViews,
		postgres.SearchLatencyDistribution,
		postgres.SearchResponseCount,
		frontend.FetchLatencyDistribution,
		frontend.FetchResponseCount,
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
		middleware.BetaPkgGoDevRedirect(),
		middleware.Quota(cfg.Quota, cacheClient),
		middleware.SecureHeaders(!*disableCSP), // must come before any caching for nonces to work
		middleware.Experiment(experimenter),
		middleware.Panic(panicHandler),
		ermw,
		middleware.Timeout(54*time.Second),
	)
	addr := cfg.HostAddr(*hostAddr)
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, mw(router)))
}
