// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The seeddb command is used to populates a database with an initial set of
// modules.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/dynconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/worker"
	"golang.org/x/sync/errgroup"
)

var (
	seedfile = flag.String("seed", "devtools/cmd/seeddb/seed.txt", "filename containing modules for seeding the database; relative to devtools/cmd/seeddb")
)

func main() {
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}

	exps, err := fetchExperiments(ctx, cfg)
	if err != nil {
		log.Fatal(ctx, err)
	}
	ctx = experiment.NewContext(ctx, exps...)

	// Wrap the postgres driver with our own wrapper, which adds OpenCensus instrumentation.
	ddb, err := database.Open("pgx", cfg.DBConnInfo(), "seeddb")
	if err != nil {
		log.Fatalf(ctx, "database.Open for host %s failed with %v", cfg.DBHost, err)
	}
	db := postgres.New(ddb)
	defer db.Close()

	if err := run(ctx, db, cfg.ProxyURL); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, db *postgres.DB, proxyURL string) error {
	start := time.Now()

	proxyClient, err := proxy.New(proxyURL)
	if err != nil {
		return err
	}

	sourceClient := source.NewClient(config.SourceTimeout)
	seedModules, err := readSeedFile(ctx, *seedfile)
	if err != nil {
		return err
	}

	r := results{}
	g := new(errgroup.Group)
	f := &worker.Fetcher{
		ProxyClient:  proxyClient,
		SourceClient: sourceClient,
		DB:           db,
	}
	for _, m := range seedModules {
		m := m
		vers := []string{m.version}
		if m.version == "all" {
			if m.path == stdlib.ModulePath {
				stdVersions, err := stdlib.Versions()
				if err != nil {
					return err
				}
				// As an optimization, only fetch release versions for the
				// standard library.
				vers = nil
				for _, v := range stdVersions {
					if strings.HasSuffix(v, ".0") {
						vers = append(vers, v)
					}
				}
			} else {
				vers, err = proxyClient.Versions(ctx, m.path)
				if err != nil {
					return err
				}
			}
		}
		for _, v := range vers {
			v := v
			g.Go(func() error {
				// Log the duration of this fetch request.
				start := time.Now()
				defer func() {
					r.add(m.path, v, start)
				}()

				return fetchFunc(ctx, f, m.path, v)
			})
		}
	}
	if err := g.Wait(); err != nil {
		return err
	}
	log.Infof(ctx, "Successfully fetched all modules: %v", time.Since(start))

	// Print the time it took to fetch these modules.
	var keys []string
	for k := range r.paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Infof(ctx, "%s | %v", k, r.paths[k])
	}
	return nil
}

func fetchFunc(ctx context.Context, f *worker.Fetcher, m, v string) (err error) {
	defer derrors.Wrap(&err, "fetchFunc(ctx, f, %q, %q)", m, v)

	log.Infof(ctx, "Fetch requested: %q %q", m, v)
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	code, _, err := f.FetchAndUpdateState(fetchCtx, m, v, "")
	if err != nil {
		if code == http.StatusNotFound {
			// We expect
			// github.com/jackc/pgx/pgxpool@v3.6.2+incompatible
			// to fail from seed.txt, so that it will redirect to
			// github.com/jackc/pgx/v4/pgxpool in tests.
			return nil
		}
		return err
	}
	return nil
}

type results struct {
	paths map[string]time.Duration
	mu    sync.Mutex
}

func (r *results) add(modPath, version string, start time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.paths == nil {
		r.paths = map[string]time.Duration{}
	}
	r.paths[fmt.Sprintf("%s@%s", modPath, version)] = time.Since(start)
}

type module struct {
	path    string
	version string
}

// readSeedFile reads a file of module versions that we want to fetch for
// seeding the database. Format of the file:
// each line is
//     module@version
func readSeedFile(ctx context.Context, seedfile string) (_ []*module, err error) {
	defer derrors.Wrap(&err, "readSeedFile %q", seedfile)
	lines, err := readFileLines(seedfile)
	if err != nil {
		return nil, err
	}
	log.Infof(ctx, "read %d module versions from %s", len(lines), seedfile)

	var modules []*module
	for _, l := range lines {
		parts := strings.SplitN(l, "@", 2)
		modules = append(modules, &module{
			path:    parts[0],
			version: parts[1],
		})
	}
	return modules, nil
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

func fetchExperiments(ctx context.Context, cfg *config.Config) ([]string, error) {
	if cfg.DynamicConfigLocation == "" {
		return nil, nil
	}
	dc, err := dynconfig.Read(ctx, cfg.DynamicConfigLocation)
	if err != nil {
		return nil, err
	}
	var exps []string
	for _, e := range dc.Experiments {
		if e.Rollout > 0 {
			exps = append(exps, e.Name)
		}
	}
	return exps, nil
}
