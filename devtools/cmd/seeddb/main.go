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
	"log"
	"net/http"
	"regexp"
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
	log.SetFlags(0)
	log.SetPrefix("seeddb: ")
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

	connInfo := cfg.DBConnInfo()
	db, err := database.Open("pgx", connInfo, "seeddb")
	if err != nil {
		log.Fatalf("database.Open for host %s failed with %v", cfg.DBHost, err)
	}
	defer db.Close()
	log.Printf("connected to %s", redactPassword(connInfo))

	if err := run(ctx, db, cfg.ProxyURL); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, db *database.DB, proxyURL string) error {
	start := time.Now()

	proxyClient, err := proxy.New(proxyURL, new(ochttp.Transport))
	if err != nil {
		return err
	}

	sourceClient := source.NewClient(&http.Client{
		Transport: new(ochttp.Transport),
		Timeout:   config.SourceTimeout,
	})
	seedModules, err := readSeedFile(*seedfile)
	if err != nil {
		return err
	}

	var (
		mu     sync.Mutex
		errors database.MultiErr
	)

	// Expand versions and group by module path.
	log.Printf("expanding versions")
	versionsByPath := map[string][]string{}
	for _, m := range seedModules {
		vers, err := versions(ctx, proxyClient, m)
		if err != nil {
			if *keepGoing {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			} else {
				return err
			}
		}
		versionsByPath[m.Path] = append(versionsByPath[m.Path], vers...)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)
	f := &worker.Fetcher{
		ProxyClient:  proxyClient,
		SourceClient: sourceClient,
	}
	if *bypassLicenseCheck {
		f.DB = postgres.NewBypassingLicenseCheck(db)
	} else {
		f.DB = postgres.New(db)
	}

	log.Printf("fetching")
	for path, vers := range versionsByPath {
		path := path
		vers := vers
		// Process versions of the same module sequentially, to avoid DB contention.
		g.Go(func() error {
			for _, v := range vers {
				if err := fetch(gctx, db, f, path, v); err != nil {
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
		log.Printf("Wait failed: %v", err)
		return err
	}
	if len(errors) > 0 {
		log.Printf("there were errors")
		return errors
	}
	log.Printf("successfully fetched all modules in %s", time.Since(start).Round(time.Millisecond))
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

func fetch(ctx context.Context, db *database.DB, f *worker.Fetcher, m, v string) error {
	// Record the duration of this fetch request.
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT 1 FROM modules WHERE module_path = $1 AND version = $2;
	`, m, v).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) || *refetch {
		return fetchFunc(ctx, f, m, v)
	}
	log.Printf("%s@%s exists", m, v)
	return nil
}

func fetchFunc(ctx context.Context, f *worker.Fetcher, m, v string) (err error) {
	defer derrors.Wrap(&err, "fetchFunc(ctx, f, %q, %q)", m, v)

	fetchCtx, cancel := context.WithTimeout(ctx, 7*time.Minute)
	defer cancel()

	start := time.Now()
	code, _, err := f.FetchAndUpdateState(fetchCtx, m, v, "")
	log.Printf("%s@%s fetched in %s", m, v, time.Since(start).Round(time.Millisecond))
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

// readSeedFile reads a file of module versions that we want to fetch for
// seeding the database. Each line of the file should be of the form:
//
//	module@version
func readSeedFile(seedfile string) (_ []internal.Modver, err error) {
	defer derrors.Wrap(&err, "readSeedFile %q", seedfile)
	lines, err := internal.ReadFileLines(seedfile)
	if err != nil {
		return nil, err
	}
	log.Printf("read %d module versions from %s", len(lines), seedfile)

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

var passwordRegexp = regexp.MustCompile(`password=\S+`)

func redactPassword(dbinfo string) string {
	return passwordRegexp.ReplaceAllLiteralString(dbinfo, "password=REDACTED")
}
