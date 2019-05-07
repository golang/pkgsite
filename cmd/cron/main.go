// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/discovery/internal/cron"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/secrets"
)

var (
	indexURL   = getEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index")
	fetchURL   = getEnv("GO_DISCOVERY_FETCH_URL", "http://localhost:9000")
	timeout    = getEnv("GO_DISCOVERY_CRON_TIMEOUT_MINUTES", "10")
	workers    = flag.Int("workers", 10, "number of concurrent requests to the fetch service")
	staticPath = flag.String("static", "content/static", "path to folder containing static files served")
)

func main() {
	flag.Parse()

	ctx := context.Background()
	dbinfo, err := dbConnInfo(ctx)
	if err != nil {
		log.Fatalf("Unable to construct database connection info string: %v", err)
	}
	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open(%q): %v", dbinfo, err)
	}
	defer db.Close()

	indexClient, err := index.New(indexURL)
	if err != nil {
		log.Fatalf("index.New(%q): %v", indexURL, err)
	}

	fetchClient := fetch.New(fetchURL)

	templatePath := filepath.Join(*staticPath, "html/cron/index.tmpl")
	indexTemplate, err := template.New("index.tmpl").ParseFiles(templatePath)
	if err != nil {
		log.Fatalf("template.ParseFiles(%q): %v", templatePath, err)
	}

	handlerTimeout, err := strconv.Atoi(timeout)
	if err != nil {
		log.Fatalf("strconv.Atoi(%q): %v", timeout, err)
	}

	server := cron.NewServer(db, indexClient, fetchClient, indexTemplate, *workers)

	// Default to addr on localhost to mute security popup about incoming
	// network connections when running locally. When running in prod, App
	// Engine requires that the app listens on the port specified by the
	// environment variable PORT.
	var addr string
	if port := os.Getenv("PORT"); port != "" {
		addr = fmt.Sprintf(":%s", port)
	} else {
		addr = "localhost:8000"
	}

	mw := middleware.Timeout(time.Duration(handlerTimeout) * time.Minute)
	log.Printf("Listening on addr %s", addr)
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
		password, err = secrets.Get(ctx, "go_discovery_database_password_proxy_index_cron")
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
