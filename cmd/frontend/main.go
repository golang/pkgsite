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
	"go.opencensus.io/plugin/ochttp"
	octrace "go.opencensus.io/trace"
	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/frontend/fetchserver"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/middleware/timeout"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/queue/gcpqueue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/static"
	"golang.org/x/pkgsite/internal/trace"
	"golang.org/x/pkgsite/internal/vuln"
)

var (
	queueName      = serverconfig.GetEnv("GO_DISCOVERY_FRONTEND_TASK_QUEUE", "")
	workers        = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	staticFlag     = flag.String("static", "static", "path to folder containing static files served")
	thirdPartyPath = flag.String("third_party", "third_party", "path to folder containing third-party libraries")
	devMode        = flag.Bool("dev", false, "enable developer mode (reload templates on each page load, serve non-minified JS/CSS, etc.)")
	localMode      = flag.Bool("local", false, "enable local mode (hide irrelevant content and links to go.dev)")
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
	cfg, err := serverconfig.Init(ctx)
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

	proxyClient, err := proxy.New(*proxyURL, &ochttp.Transport{})
	if err != nil {
		log.Fatal(ctx, err)
	}

	if *directProxy {
		sourceClient := source.NewClient(&http.Client{Transport: &ochttp.Transport{}, Timeout: 1 * time.Minute})
		ds := fetchdatasource.Options{
			Getters: []fetch.ModuleGetter{
				fetch.NewProxyModuleGetter(proxyClient, sourceClient),
				fetch.NewStdlibZipModuleGetter(),
			},
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
		sourceClient := source.NewClient(&http.Client{
			Transport: new(ochttp.Transport),
			Timeout:   config.SourceTimeout,
		})
		// The closure passed to queue.New is only used for testing and local
		// execution, not in production. So it's okay that it doesn't use a
		// per-request connection.
		fetchQueue, err = gcpqueue.New(ctx, cfg, queueName, *workers, expg,
			func(ctx context.Context, modulePath, version string) (int, error) {
				return fetchserver.FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, db)
			})
		if err != nil {
			log.Fatalf(ctx, "gcpqueue.New: %v", err)
		}
	}

	trace.SetTraceFunction(func(ctx context.Context, name string) (context.Context, trace.Span) {
		return octrace.StartSpan(ctx, name)
	})
	reporter := cmdconfig.Reporter(ctx, cfg)
	vc, err := vuln.NewClient(cfg.VulnDB)
	if err != nil {
		log.Fatalf(ctx, "vuln.NewClient: %v", err)
	}
	staticSource := template.TrustedSourceFromFlag(flag.Lookup("static").Value)
	if *devMode {
		// In dev mode compile TypeScript files into minified JavaScript files
		// and rebuild them on file changes.
		if *staticFlag == "" {
			panic("staticPath is empty in dev mode; cannot rebuild static files")
		}
		ctx := context.Background()
		if err := static.Build(static.Config{
			EntryPoint: *staticFlag + "/frontend",
			Watch:      true,
			Bundle:     true,
		}); err != nil {
			log.Error(ctx, err)
		}
	}

	// TODO: Can we use a separate queue for the fetchServer and for the Server?
	// It would help differentiate ownership.
	fetchServer := &fetchserver.FetchServer{
		Queue:                fetchQueue,
		TaskIDChangeInterval: config.TaskIDChangeIntervalFrontend,
	}
	server, err := frontend.NewServer(frontend.ServerConfig{
		Config:            cfg,
		FetchServer:       fetchServer,
		DataSourceGetter:  dsg,
		Queue:             fetchQueue,
		TemplateFS:        template.TrustedFSFromTrustedSource(staticSource),
		StaticFS:          os.DirFS(*staticFlag),
		ThirdPartyFS:      os.DirFS(*thirdPartyPath),
		DevMode:           *devMode,
		LocalMode:         *localMode,
		Reporter:          reporter,
		VulndbClient:      vc,
		DepsDevHTTPClient: &http.Client{Transport: new(ochttp.Transport)},
	})
	if err != nil {
		log.Fatalf(ctx, "frontend.NewServer: %v", err)
	}

	router := dcensus.NewRouter(frontend.TagRoute)
	var redisClient *redis.Client
	var cacher frontend.Cacher
	if cfg.RedisCacheHost != "" {
		addr := cfg.RedisCacheHost + ":" + cfg.RedisCachePort
		redisClient = redis.NewClient(&redis.Options{Addr: addr})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Errorf(ctx, "redis at %s: %v", addr, err)
		} else {
			log.Infof(ctx, "connected to redis at %s", addr)
		}
		cacher = middleware.NewCacher(redisClient)
	}
	server.Install(router.Handle, cacher, cfg.AuthValues)
	views := append(dcensus.ServerViews,
		postgres.SearchLatencyDistribution,
		postgres.SearchResponseCount,
		fetchserver.FetchLatencyDistribution,
		fetchserver.FetchResponseCount,
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
	if !serverconfig.OnAppEngine() {
		dcensusServer, err := dcensus.DebugHandler()
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
	experimenter := cmdconfig.Experimenter(ctx, cfg, expg, reporter)
	log.Infof(ctx, "cmd/frontend: initialized cmdconfig.Experimenter")

	ermw := middleware.Identity()
	if reporter != nil {
		ermw = middleware.ErrorReporting(reporter)
	}
	mw := middleware.Chain(
		middleware.RequestInfo(),
		middleware.RequestLog(cmdconfig.Logger(ctx, cfg, "frontend-log")),
		middleware.AcceptRequests(http.MethodGet, http.MethodPost, http.MethodHead), // accept only GETs, POSTs and HEADs
		middleware.GodocOrgRedirect(),
		middleware.Quota(cfg.Quota, redisClient),
		middleware.SecureHeaders(!*disableCSP), // must come before any caching for nonces to work
		middleware.Experiment(experimenter),
		middleware.Panic(panicHandler),
		ermw,
		timeout.Timeout(54*time.Second),
	)
	addr := cfg.HostAddr(*hostAddr)
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, mw(router)))
}
