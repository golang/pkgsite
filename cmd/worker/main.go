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

	"cloud.google.com/go/errorreporting"
	"cloud.google.com/go/profiler"
	"github.com/go-redis/redis/v7"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/worker"

	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"

	"contrib.go.opencensus.io/integrations/ocsql"
)

var (
	timeout   = config.GetEnv("GO_DISCOVERY_WORKER_TIMEOUT_MINUTES", "10")
	queueName = config.GetEnv("GO_DISCOVERY_WORKER_TASK_QUEUE", "")
	workers   = flag.Int("workers", 10, "number of concurrent requests to the fetch service, when running locally")
	// flag used in call to safehtml/template.TrustedSourceFromFlag
	_ = flag.String("static", "content/static", "path to folder containing static files served")
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

	readProxyRemoved(ctx)

	// Wrap the postgres driver with OpenCensus instrumentation.
	driverName, err := ocsql.Register("postgres", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Fatalf(ctx, "unable to register the ocsql driver: %v\n", err)
	}
	ddb, err := database.Open(driverName, cfg.DBConnInfo(), cfg.InstanceID)
	if err != nil {
		log.Fatalf(ctx, "database.Open: %v", err)
	}
	db := postgres.New(ddb)
	defer db.Close()

	populateExcluded(ctx, db)

	indexClient, err := index.New(cfg.IndexURL)
	if err != nil {
		log.Fatal(ctx, err)
	}
	proxyClient, err := proxy.New(cfg.ProxyURL)
	if err != nil {
		log.Fatal(ctx, err)
	}
	sourceClient := source.NewClient(config.SourceTimeout)
	fetchQueue, err := queue.New(ctx, cfg, queueName, *workers, db,
		func(ctx context.Context, modulePath, version string) (int, error) {
			return worker.FetchAndUpdateState(ctx, modulePath, version, proxyClient, sourceClient, db, cfg.AppVersionLabel())
		})
	if err != nil {
		log.Fatalf(ctx, "queue.New: %v", err)
	}

	reportingClient := reportingClient(ctx, cfg)
	redisHAClient := getHARedis(ctx, cfg)
	redisCacheClient := getCacheRedis(ctx, cfg)
	server, err := worker.NewServer(cfg, worker.ServerConfig{
		DB:                   db,
		IndexClient:          indexClient,
		ProxyClient:          proxyClient,
		SourceClient:         sourceClient,
		RedisHAClient:        redisHAClient,
		RedisCacheClient:     redisCacheClient,
		Queue:                fetchQueue,
		ReportingClient:      reportingClient,
		TaskIDChangeInterval: config.TaskIDChangeIntervalWorker,
		StaticPath:           template.TrustedSourceFromFlag(flag.Lookup("static").Value),
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	router := dcensus.NewRouter(nil)
	server.Install(router.Handle)

	views := append(dcensus.ClientViews, dcensus.ServerViews...)
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
		go http.ListenAndServe(cfg.DebugAddr("localhost:8001"), dcensusServer)
	}

	handlerTimeout, err := strconv.Atoi(timeout)
	if err != nil {
		log.Fatalf(ctx, "strconv.Atoi(%q): %v", timeout, err)
	}
	requestLogger := logger(ctx, cfg)

	experimenter, err := middleware.NewExperimenter(ctx, 1*time.Minute, db)
	if err != nil {
		log.Fatal(ctx, err)
	}
	mw := middleware.Chain(
		middleware.RequestLog(requestLogger),
		middleware.Timeout(time.Duration(handlerTimeout)*time.Minute),
		middleware.Experiment(experimenter),
	)
	http.Handle("/", mw(router))

	addr := cfg.HostAddr("localhost:8000")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}

func getHARedis(ctx context.Context, cfg *config.Config) *redis.Client {
	// We update completions with one big pipeline, so we need long write
	// timeouts. ReadTimeout is increased only to be consistent with
	// WriteTimeout.
	return getRedis(ctx, cfg.RedisHAHost, cfg.RedisHAPort, 5*time.Minute, 5*time.Minute)
}

func getCacheRedis(ctx context.Context, cfg *config.Config) *redis.Client {
	return getRedis(ctx, cfg.RedisCacheHost, cfg.RedisCachePort, 0, 0)
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

func reportingClient(ctx context.Context, cfg *config.Config) *errorreporting.Client {
	if !cfg.OnAppEngine() {
		return nil
	}
	reporter, err := errorreporting.NewClient(ctx, cfg.ProjectID, errorreporting.Config{
		ServiceName: cfg.ServiceID,
		OnError: func(err error) {
			log.Errorf(ctx, "Error reporting failed: %v", err)
		},
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	return reporter
}

func logger(ctx context.Context, cfg *config.Config) middleware.Logger {
	if cfg.OnAppEngine() {
		logger, err := log.UseStackdriver(ctx, cfg, "worker-log")
		if err != nil {
			log.Fatal(ctx, err)
		}
		return logger
	}
	return middleware.LocalLogger{}
}

// Read a file of module versions that we should ignore because
// the are in the index but not stored in the proxy.
// Format of the file: each line is
//     module@version
func readProxyRemoved(ctx context.Context) {
	filename := config.GetEnv("GO_DISCOVERY_PROXY_REMOVED", "")
	if filename == "" {
		return
	}
	lines, err := readFileLines(filename)
	if err != nil {
		log.Fatal(ctx, err)
	}
	for _, line := range lines {
		worker.ProxyRemoved[line] = true
	}
	log.Infof(ctx, "read %d excluded module versions from %s", len(worker.ProxyRemoved), filename)
}

// populateExcluded adds each element of excludedPrefixes to the excluded_prefixes
// table if it isn't already present.
func populateExcluded(ctx context.Context, db *postgres.DB) {
	filename := config.GetEnv("GO_DISCOVERY_EXCLUDED_FILENAME", "")
	if filename == "" {
		return
	}
	lines, err := readFileLines(filename)
	if err != nil {
		log.Fatal(ctx, err)
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "etl"
	}
	for _, line := range lines {
		var prefix, reason string
		i := strings.IndexAny(line, " \t")
		if i >= 0 {
			prefix = line[:i]
			reason = strings.TrimSpace(line[i+1:])
		}
		if reason == "" {
			log.Fatalf(ctx, "missing reason in %s, line %q", filename, line)
		}
		present, err := db.IsExcluded(ctx, prefix)
		if err != nil {
			log.Fatalf(ctx, "db.IsExcluded(%q): %v", prefix, err)
		}
		if !present {
			if err := db.InsertExcludedPrefix(ctx, prefix, user, reason); err != nil {
				log.Fatalf(ctx, "db.InsertExcludedPrefix(%q, %q, %q): %v", prefix, user, reason, err)
			}
		}
	}
}

// readFileLines reads filename and returns its lines, trimmed of whitespace.
// Blank lines and lines whose first non-blank character is '#' are omitted.
func readFileLines(filename string) ([]string, error) {
	var lines []string
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		lines = append(lines, line)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
