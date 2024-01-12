// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The seeddb command is used to populates a database with an initial set of
// modules.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/log"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: db [cmd]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  create: creates a new database. It does not run migrations\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  migrate: runs all migrations \n")
		fmt.Fprintf(flag.CommandLine.Output(), "  drop: drops database\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  truncate: truncates all tables in database\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  recreate: drop, create and run migrations\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Database name is set using $GO_DISCOVERY_DATABASE_NAME. ")
		fmt.Fprintf(flag.CommandLine.Output(), "See doc/postgres.md for details.\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.SetLevel("info")

	dbName := serverconfig.GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-db")
	if err := run(ctx, flag.Args()[0], dbName, cfg.DBConnInfo()); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, cmd, dbName, connectionInfo string) error {
	switch cmd {
	case "create":
		return create(ctx, dbName)
	case "migrate":
		return migrate(dbName)
	case "drop":
		return drop(ctx, dbName)
	case "recreate":
		return recreate(ctx, dbName)
	case "truncate":
		return truncate(ctx, connectionInfo)
	case "waiting":
		return waiting(ctx, connectionInfo)
	default:
		return fmt.Errorf("unsupported arg: %q", cmd)
	}
}

func create(ctx context.Context, dbName string) error {
	if err := database.CreateDBIfNotExists(dbName); err != nil {
		if strings.HasSuffix(err.Error(), "already exists") {
			// The error will have the format:
			// error creating "discovery-db": pq: database "discovery-db" already exists
			// Trim the beginning to make it clear that this is not an error
			// that matters.
			log.Debugf(ctx, strings.TrimPrefix(err.Error(), "error creating "))
			return nil
		}
		return err
	}
	return nil
}

func migrate(dbName string) error {
	_, err := database.TryToMigrate(dbName)
	return err
}

func drop(ctx context.Context, dbName string) error {
	err := database.DropDB(dbName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			// The error will have the format:
			// ...server error (FATAL: database "discovery-dbasdasdas" does not exist (SQLSTATE 3D000))
			// or
			// error dropping "discovery_frontend_test": pq: database "discovery_frontend_test" does not exist
			log.Infof(ctx, "Database does not exist: %q", dbName)
			return nil
		}
		return err
	}
	log.Infof(ctx, "Dropped database: %q", dbName)
	return nil
}

func recreate(ctx context.Context, dbName string) error {
	if err := drop(ctx, dbName); err != nil {
		return err
	}
	if err := database.CreateDB(dbName); err != nil {
		return err
	}
	return migrate(dbName)
}

func truncate(ctx context.Context, connectionInfo string) error {
	// Wrap the postgres driver with our own wrapper, which adds OpenCensus instrumentation.
	ddb, err := database.Open("pgx", connectionInfo, "dbadmin")
	if err != nil {
		return err
	}
	defer ddb.Close()
	return database.ResetDB(ctx, ddb)
}

type ProcessInfo struct {
	pid           int64
	start         time.Time
	state         string
	waitEventType *string
	waitEvent     *string
	blockingPIDs  []int64
	query         string
	pos           int
}

func waiting(ctx context.Context, connectionInfo string) error {
	var processInfos []*ProcessInfo
	db, err := database.Open("pgx", connectionInfo, "dbadmin")
	if err != nil {
		return err
	}
	defer db.Close()

	query := `
		SELECT pid, query_start, state, wait_event_type, wait_event, pg_blocking_pids(pid), query
		FROM pg_stat_activity
		WHERE usename='worker'
		ORDER BY 2
	`

	err = db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var pi ProcessInfo
		if err := rows.Scan(&pi.pid, &pi.start, &pi.state, &pi.waitEventType, &pi.waitEvent, pq.Array(&pi.blockingPIDs), &pi.query); err != nil {
			return err
		}
		processInfos = append(processInfos, &pi)
		return nil
	})
	if err != nil {
		return err
	}

	byPid := map[int64]*ProcessInfo{}
	for _, pi := range processInfos {
		byPid[pi.pid] = pi
	}
	sorted := topoSort(processInfos, byPid)
	for i, p := range sorted {
		p.pos = i + 1
	}
	for _, pi := range sorted {
		var wps []int
		for _, w := range pi.blockingPIDs {
			wps = append(wps, byPid[w].pos)
		}
		pi.query = strings.TrimSpace(pi.query)
		pi.query = strings.Join(strings.Fields(pi.query), " ")
		secs := time.Since(pi.start).Seconds()
		mins := int(secs / 60)
		secs -= float64(mins) * 60
		fmt.Printf("%3d %d  %2d:%2.3fs %v\t %s\n", pi.pos, pi.pid, mins, secs, wps, left(pi.query, 50))
	}
	return nil
}

func left(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func topoSort(items []*ProcessInfo, byPid map[int64]*ProcessInfo) []*ProcessInfo {
	var res []*ProcessInfo

	visited := map[*ProcessInfo]bool{}
	var visit func(*ProcessInfo)
	visit = func(pi *ProcessInfo) {
		if visited[pi] {
			return
		}
		visited[pi] = true
		for _, bpid := range pi.blockingPIDs {
			visit(byPid[bpid])
		}
		res = append(res, pi)

	}
	for _, it := range items {
		if !visited[it] {
			visit(it)
		}
	}

	return res
}
