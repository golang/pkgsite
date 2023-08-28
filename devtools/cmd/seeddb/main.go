// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The seeddb command populates a database with an initial set of modules.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/dynconfig"
	"golang.org/x/pkgsite/internal/config/serverconfig"
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
	seedfile           = flag.String("seed", "devtools/cmd/seeddb/seed.txt", "filename containing modules for seeding the database")
	refetch            = flag.Bool("refetch", false, "refetch modules in the seedfile even if they already exist")
	keepGoing          = flag.Bool("keep_going", false, "continue on errors")
	bypassLicenseCheck = flag.Bool("bypass_license_check", false,
		"insert all data into the DB, even for non-redistributable paths")
)

func main() {
	flag.Parse()

	ctx := context.Background()
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}

	exps, err := fetchExperiments(ctx, cfg)
	if err != nil {
		log.Fatal(ctx, err)
	}
	ctx = experiment.NewContext(ctx, exps...)

	db, err := database.Open("pgx", cfg.DBConnInfo(), "seeddb")
	if err != nil {
		log.Fatalf(ctx, "database.Open for host %s failed with %v", cfg.DBHost, err)
	}
	defer db.Close()

	if err := run(ctx, db, cfg.ProxyURL); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, db *database.DB, proxyURL string) error {
	start := time.Now()

	proxyClient, err := proxy.New(proxyURL)
	if err != nil {
		return err
	}

	sourceClient := source.NewClient(&http.Client{
		Transport: &ochttp.Transport{},
		Timeout:   config.SourceTimeout,
	})
	seedModules, err := readSeedFile(ctx, *seedfile)
	if err != nil {
		return err
	}

	// Expand versions and group by module path.
	versionsByPath := map[string][]string{}
	for _, m := range seedModules {
		vers, err := versions(ctx, proxyClient, m)
		if err != nil {
			return err
		}
		versionsByPath[m.Path] = append(versionsByPath[m.Path], vers...)
	}

	r := results{}
	g := new(errgroup.Group)
	f := &worker.Fetcher{
		ProxyClient:  proxyClient,
		SourceClient: sourceClient,
	}
	if *bypassLicenseCheck {
		f.DB = postgres.NewBypassingLicenseCheck(db)
	} else {
		f.DB = postgres.New(db)
	}

	var (
		mu     sync.Mutex
		errors database.MultiErr
	)
	for path, vers := range versionsByPath {
		path := path
		vers := vers
		// Process versions of the same module sequentially, to avoid DB contention.
		g.Go(func() error {
			for _, v := range vers {
				if err := fetch(ctx, db, f, path, v, &r); err != nil {
					if *keepGoing {
						mu.Lock()
						errors = append(errors, err)
						mu.Unlock()
					} else {
						return err
					}
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if len(errors) > 0 {
		return errors
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

func versions(ctx context.Context, proxyClient *proxy.Client, mv internal.Modver) ([]string, error) {
	if mv.Version != "all" {
		return []string{mv.Version}, nil
	}
	if mv.Path == stdlib.ModulePath {
		stdVersions, err := stdlib.Versions()
		if err != nil {
			return nil, err
		}
		// As an optimization, only fetch release versions for the standard
		// library.
		var vers []string
		for _, v := range stdVersions {
			if strings.HasSuffix(v, ".0") {
				vers = append(vers, v)
			}
		}
		return vers, nil
	}
	return proxyClient.Versions(ctx, mv.Path)
}

func fetch(ctx context.Context, db *database.DB, f *worker.Fetcher, m, v string, r *results) error {
	// Record the duration of this fetch request.
	start := time.Now()

	var exists bool
	defer func() {
		r.add(m, v, start, exists)
	}()
	err := db.QueryRow(ctx, `
		SELECT 1 FROM modules WHERE module_path = $1 AND version = $2;
	`, m, v).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) || *refetch {
		return fetchFunc(ctx, f, m, v)
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
	mu    sync.Mutex
	paths map[string]time.Duration
}

func (r *results) add(modPath, version string, start time.Time, exists bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.paths == nil {
		r.paths = map[string]time.Duration{}
	}
	key := fmt.Sprintf("%s@%s", modPath, version)
	if exists {
		key = fmt.Sprintf("%s (exists)", key)
	}
	r.paths[key] = time.Since(start)
}

// readSeedFile reads a file of module versions that we want to fetch for
// seeding the database. Each line of the file should be of the form:
//
//	module@version
func readSeedFile(ctx context.Context, seedfile string) (_ []internal.Modver, err error) {
	defer derrors.Wrap(&err, "readSeedFile %q", seedfile)
	lines, err := internal.ReadFileLines(seedfile)
	if err != nil {
		return nil, err
	}
	log.Infof(ctx, "read %d module versions from %s", len(lines), seedfile)

	var modules []internal.Modver
	for _, l := range lines {
		mv, err := internal.ParseModver(l)
		if err != nil {
			return nil, err
		}
		modules = append(modules, mv)
	}
	return modules, nil
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
