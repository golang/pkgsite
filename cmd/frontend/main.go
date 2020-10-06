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
	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-redis/redis/v7"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend"
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
	disableCSP     = flag.Bool("nocsp", false, "enable Content Security Policy")
	proxyURL       = flag.String("proxy_url", "https://proxy.golang.org", "Uses the module proxy referred to by this URL "+
		"for direct proxy mode and frontend fetches")
	directProxy = flag.Bool("direct_proxy", false, "if set to true, uses the module proxy referred to by this URL "+
		"as a direct backend, bypassing the database")
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
	proxyClient, err := proxy.New(*proxyURL)
	if err != nil {
		log.Fatal(ctx, err)
	}
	if *bypassLicenseCheck {
		log.Info(ctx, "BYPASSING LICENSE CHECKING: DISPLAYING NON-REDISTRIBUTABLE INFORMATION")
	}
	expg := cmdconfig.ExperimentGetter(ctx, cfg)
	if *directProxy {
		var pds *proxydatasource.DataSource
		if *bypassLicenseCheck {
			pds = proxydatasource.NewBypassingLicenseCheck(proxyClient)
		} else {
			pds = proxydatasource.New(proxyClient)
		}
		dsg = func(context.Context) internal.DataSource { return pds }
	} else {
		// Wrap the postgres driver with OpenCensus instrumentation.
		ocDriver, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
		if err != nil {
			log.Fatalf(ctx, "unable to register the ocsql driver: %v\n", err)
		}
		ddb, err := openDB(ctx, cfg, ocDriver)
		if err != nil {
			log.Fatal(ctx, err)
		}
		var db *postgres.DB
		if *bypassLicenseCheck {
			db = postgres.NewBypassingLicenseCheck(ddb)
		} else {
			db = postgres.New(ddb)
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
	experimenter := cmdconfig.Experimenter(ctx, cfg, expg, rc)
	ermw := middleware.Identity()
	if rc != nil {
		ermw = middleware.ErrorReporting(rc.Report)
	}
	mw := middleware.Chain(
		middleware.RequestLog(cmdconfig.Logger(ctx, cfg, "frontend-log")),
		middleware.AcceptRequests(http.MethodGet, http.MethodPost), // accept only GETs and POSTs
		middleware.Quota(cfg.Quota),
		middleware.GodocURL(),                                                                 // potentially redirects so should be early in chain
		middleware.SecureHeaders(!*disableCSP),                                                // must come before any caching for nonces to work
		middleware.LatestVersions(server.GetLatestMinorVersion, server.GetLatestMajorVersion), // must come before caching for version badge to work
		middleware.Panic(panicHandler),
		ermw,
		middleware.Timeout(54*time.Second),
		middleware.Experiment(experimenter),
	)
	addr := cfg.HostAddr("localhost:8080")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, mw(router)))
}

// TODO(https://github.com/golang/go/issues/40097): factor out to reduce
// duplication with cmd/worker/main.go.

// openDB opens a connection to a database with the given driver, using connection info from
// the given config.
// It first tries the main connection info (DBConnInfo), and if that fails, it uses backup
// connection info it if exists (DBSecondaryConnInfo).
func openDB(ctx context.Context, cfg *config.Config, driver string) (_ *database.DB, err error) {
	defer derrors.Wrap(&err, "openDB(ctx, cfg, %q)", driver)
	log.Infof(ctx, "opening database on host %s", cfg.DBHost)
	ddb, err := database.Open(driver, cfg.DBConnInfo(), cfg.InstanceID)
	if err == nil {
		return ddb, nil
	}
	ci := cfg.DBSecondaryConnInfo()
	if ci == "" {
		log.Infof(ctx, "no secondary DB host")
		return nil, err
	}
	log.Errorf(ctx, "database.Open for primary host %s failed with %v; trying secondary host %s ",
		cfg.DBHost, err, cfg.DBSecondaryHost)
	return database.Open(driver, ci, cfg.InstanceID)
}
