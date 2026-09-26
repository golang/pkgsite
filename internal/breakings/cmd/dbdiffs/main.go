// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The dbdiffs command connects to a pkgsite database and prints the breaking
// changes between consecutive versions of a module.
//
// Only release versions at v1 or higher are considered, and only packages that
// other modules can import: commands and internal packages are ignored.
//
// The database connection is configured with the GO_DISCOVERY_DATABASE_*
// environment variables; see doc/postgres.md.
package main

import (
	"cmp"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // for pgx driver
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/breakings"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/godoc"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("dbdiffs: ")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s MODULE_PATH\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "The database is configured with the GO_DISCOVERY_DATABASE_* environment variables. ")
		fmt.Fprintf(flag.CommandLine.Output(), "See doc/postgres.md for details.\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	ctx := context.Background()
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := run(ctx, cfg.DBConnInfo(), flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}

// A pkg is a package at a particular module version.
type pkg struct {
	path    string
	version string
	source  []byte         // encoded godoc.Package; nil once api is set
	api     *breakings.API // constructed on demand
}

// API returns the API of the package, constructing it on the first call.
func (p *pkg) API() (*breakings.API, error) {
	if p.api != nil {
		return p.api, nil
	}
	gpkg, err := godoc.DecodePackage(p.source)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: decoding package: %w", p.path, p.version, err)
	}
	api, err := breakings.NewAPIFromPackage(p.path, p.version, gpkg)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: constructing API: %w", p.path, p.version, err)
	}
	p.api = api
	p.source = nil
	return api, nil
}

func run(ctx context.Context, dbConnInfo, modulePath string) error {
	db, err := database.Open("pgx", dbConnInfo, "dbdiffs")
	if err != nil {
		return err
	}
	defer db.Close()

	nrel, versions, err := breakableVersions(ctx, db, modulePath)
	if err != nil {
		return err
	}
	if nrel == 0 {
		fmt.Printf("no release versions or nonexistent module\n")
		return nil
	}
	fmt.Printf("%d breakable versions\n", len(versions))
	if len(versions) < 2 {
		fmt.Println("nothing to compare")
		return nil
	}

	prev, err := breakablePackages(ctx, db, modulePath, versions[0])
	if err != nil {
		return err
	}
	for i := 1; i < len(versions); i++ {
		curr, err := breakablePackages(ctx, db, modulePath, versions[i])
		if err != nil {
			return err
		}
		if err := printChanges(os.Stdout, versions[i-1], versions[i], prev, curr); err != nil {
			return err
		}
		prev = curr
	}
	return nil
}

// breakableVersions returns the versions of modulePath for which a breaking
// change matters, in increasing order. Those are the release versions other
// than v0. It also returns the number of release versions in total.
//
// NOTE: This includes v2+incompatible versions. Since v2+incompatible has the same
// path as v1 versions, it makes sense to include it.
func breakableVersions(ctx context.Context, db *database.DB, modulePath string) (int, []string, error) {
	const query = `
		SELECT version
		FROM modules
		WHERE module_path = $1
		  AND version_type = 'release'
	`
	var versions []string
	nRelVersions := 0
	err := db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		nRelVersions++
		// v0 versions do not need to be compatible.
		if semver.Major(v) != "v0" {
			versions = append(versions, v)
		}
		return nil
	}, modulePath)
	if err != nil {
		return 0, nil, err
	}
	slices.SortFunc(versions, func(a, b string) int {
		// semver.Compare ignores build metadata (such as "+incompatible"),
		// so fall back to cmp.Compare for a deterministic order.
		return cmp.Or(semver.Compare(a, b), cmp.Compare(a, b))
	})
	return nRelVersions, versions, nil
}

// breakablePackages returns the packages of modulePath at version for which
// a breaking change matters, keyed by package path. Those are the packages
// that other modules can import: all but commands and internal packages.
//
// Each path maps to a single documentation source, chosen in the precedence
// order of [internal.BuildContexts] (with all/all preferred first, matching
// [internal.CompareBuildContexts]).
func breakablePackages(ctx context.Context, db *database.DB, modulePath, version string) (map[string]*pkg, error) {
	pkgs := map[string]*pkg{}
	err := db.RunQuery(ctx, breakablePackagesQuery(), func(rows *sql.Rows) error {
		var (
			path   string
			source []byte
		)
		if err := rows.Scan(&path, &source); err != nil {
			return err
		}
		if !isInternal(path) {
			pkgs[path] = &pkg{path: path, version: version, source: source}
		}
		return nil
	}, modulePath, version)
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// breakablePackagesQuery returns the SQL query for breakablePackages.
// It uses DISTINCT ON (p.path) ordered to match [internal.CompareBuildContexts]
// so Postgres returns a single deterministic documentation row per path.
func breakablePackagesQuery() string {
	var order strings.Builder
	order.WriteString("CASE\n")
	fmt.Fprintf(&order, "\t\t\t\tWHEN d.goos = '%s' AND d.goarch = '%s' THEN 0\n",
		internal.BuildContextAll.GOOS, internal.BuildContextAll.GOARCH)
	for i, bc := range internal.BuildContexts {
		fmt.Fprintf(&order, "\t\t\t\tWHEN d.goos = '%s' AND d.goarch = '%s' THEN %d\n",
			bc.GOOS, bc.GOARCH, i+1)
	}
	fmt.Fprintf(&order, "\t\t\t\tELSE %d\n\t\t\tEND", len(internal.BuildContexts)+1)
	return fmt.Sprintf(`
		SELECT DISTINCT ON (p.path) p.path, d.source
		FROM modules m
		INNER JOIN units u ON u.module_id = m.id
		INNER JOIN paths p ON p.id = u.path_id
		INNER JOIN documentation d ON d.unit_id = u.id
		WHERE m.module_path = $1
		  AND m.version = $2
		  AND u.name != 'main'
		  AND d.source IS NOT NULL
		ORDER BY
			p.path,
			%s,
			d.goos,
			d.goarch
	`, order.String())
}

// isInternal reports whether path is the import path of an internal package.
func isInternal(path string) bool {
	return slices.Contains(strings.Split(path, "/"), "internal")
}

// printChanges writes to w the breaking changes between the packages of two
// versions of a module. It writes nothing if there are none.
//
// Removing a package is a breaking change. For a package in both versions,
// the breaking changes are those between its two APIs.
func printChanges(w io.Writer, oldVersion, newVersion string, prev, curr map[string]*pkg) error {
	wroteHeader := false
	printf := func(format string, args ...any) {
		if !wroteHeader {
			fmt.Fprintf(w, "%s -> %s:\n", oldVersion, newVersion)
			wroteHeader = true
		}
		fmt.Fprintf(w, format, args...)
	}

	for _, path := range slices.Sorted(maps.Keys(prev)) {
		currPkg, ok := curr[path]
		if !ok {
			printf("  %s: package removed\n", path)
			continue
		}
		oldAPI, err := prev[path].API()
		if err != nil {
			return err
		}
		newAPI, err := currPkg.API()
		if err != nil {
			return err
		}
		lines := changes(oldAPI, newAPI)
		if len(lines) == 0 {
			continue
		}
		printf("  %s:\n", path)
		for _, line := range lines {
			printf("%s\n", line)
		}
	}
	return nil
}

// changes returns a list of formatted entries describing the changes between
// two versions of a package's API, sorted by symbol name. It returns nil unless
// at least one change is breaking.
// It does not consider call-compatible changes as breaking.
func changes(oldAPI, newAPI *breakings.API) []string {
	type change struct {
		sym  string
		kind breakings.ChangeKind
	}
	var cs []change
	hasBreaking := false
	for sym, kind := range oldAPI.Changes(newAPI) {
		if kind == breakings.ChangeBreaking {
			hasBreaking = true
		}
		cs = append(cs, change{sym, kind})
	}
	if !hasBreaking {
		return nil
	}
	slices.SortFunc(cs, func(a, b change) int {
		return cmp.Or(cmp.Compare(a.sym, b.sym), cmp.Compare(a.kind, b.kind))
	})
	var lines []string
	for _, c := range cs {
		if c.kind == breakings.ChangeBreaking {
			oldDef := cmp.Or(oldAPI.Definition(c.sym), "(none)")
			newDef := cmp.Or(newAPI.Definition(c.sym), "(none)")
			lines = append(lines, fmt.Sprintf("    %s: %v\n      old: %s\n      new: %s", c.sym, c.kind, oldDef, newDef))
		} else {
			lines = append(lines, fmt.Sprintf("    %s: %v", c.sym, c.kind))
		}
	}
	return lines
}
