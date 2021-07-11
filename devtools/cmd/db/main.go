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
		fmt.Fprintf(flag.CommandLine.Output(), "usage: db [cmd] [dbname]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  create [dbname]: creates a new database and run migrations\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  drop [dbname]: drops database\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  truncate [dbname]: truncates all tables in database\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  recreate [dbname]: drop, create and run migrations\n")
		fmt.Fprintf(flag.CommandLine.Output(), "dbname is set using $GO_DISCOVERY_DATABASE_NAME. ")
		fmt.Fprintf(flag.CommandLine.Output(), "See doc/postgres.md for details.\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.SetLevel(cfg.LogLevel)

	// Wrap the postgres driver with our own wrapper, which adds OpenCensus instrumentation.
	ddb, err := database.Open("pgx", cfg.DBConnInfo(), "dbadmin")
	if err != nil {
		log.Fatalf(ctx, "database.Open for host %s failed with %v", cfg.DBHost, err)
	}
	defer ddb.Close()

	if err := run(ctx, ddb, flag.Args()[0], flag.Args()[1]); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, db *database.DB, cmd, dbName string) error {
	switch cmd {
	case "create":
		return createDB(dbName)
	case "migrate":
		_, err := database.TryToMigrate(dbName)
		return err
	case "drop":
		err := database.DropDB(dbName)
		if err != nil && strings.HasSuffix(err.Error(), "does not exist") {
			fmt.Printf("%q does not exist\n", dbName)
			return nil
		}
		return nil
	case "recreate":
		if err := database.DropDB(dbName); err != nil {
			return err
		}
		return createDB(dbName)
	case "truncate":
		return database.ResetDB(ctx, db)
	default:
		return fmt.Errorf("unsupported arg: %q", cmd)
	}
}

func createDB(dbName string) error {
	if err := database.CreateDB(dbName); err != nil {
		return err
	}
	_, err := database.TryToMigrate(dbName)
	return err
}
