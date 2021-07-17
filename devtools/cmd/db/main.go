// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The seeddb command is used to populates a database with an initial set of
// modules.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal/config"
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
	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.SetLevel(cfg.LogLevel)

	dbName := config.GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-db")
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
