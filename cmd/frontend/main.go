// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/secrets"
)

func main() {
	var (
		staticPath      = flag.String("static", "content/static", "path to folder containing static files served")
		reloadTemplates = flag.Bool("reload_templates", false, "reload templates on each page load (to be used during development)")
	)
	flag.Parse()

	ctx := context.Background()
	dbinfo, err := dbConnInfo(ctx)
	if err != nil {
		log.Fatalf("Unable to construct database connection info string: %v", err)
	}
	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open: %v", err)
	}
	defer db.Close()

	server, err := frontend.NewServer(db, *staticPath, *reloadTemplates)
	if err != nil {
		log.Fatalf("frontend.NewServer: %v", err)
	}

	// Default to addr on localhost to prevent external connections. When running
	// in prod, App Engine requires that the app listens on the port specified by
	// the environment variable PORT.
	var addr string
	if port := os.Getenv("PORT"); port != "" {
		addr = fmt.Sprintf(":%s", port)
	} else {
		addr = "localhost:8080"
	}
	log.Printf("Listening on addr %s", addr)

	mw := middleware.Chain(
		middleware.SecureHeaders(),
		middleware.Timeout(1*time.Minute),
	)
	log.Fatal(http.ListenAndServe(addr, mw(server)))

}

func dbConnInfo(ctx context.Context) (string, error) {
	var (
		user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
		password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
		host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
		dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	)

	// When running on App Engine, the runtime sets GAE_ENV to 'standard' per
	// https://cloud.google.com/appengine/docs/standard/go111/runtime
	if os.Getenv("GAE_ENV") == "standard" {
		var err error
		password, err = secrets.Get(ctx, "go_discovery_database_password_frontend")
		if err != nil {
			return "", fmt.Errorf("could not get database password secret: %v", err)
		}
	}
	return fmt.Sprintf("user='%s' password='%s' host='%s' dbname='%s' sslmode=disable", user, password, host, dbname), nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
