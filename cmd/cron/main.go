// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"

	"golang.org/x/discovery/internal/cron"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

var (
	indexURL = getEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index")
	fetchURL = getEnv("GO_DISCOVERY_FETCH_URL", "http://localhost:9000")
	user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	timeout  = getEnv("GO_DISCOVERY_CRON_TIMEOUT_MINUTES", "10")
	dbinfo   = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, dbname)
	workers  = flag.Int("workers", 10, "number of concurrent requests to the fetch service")
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func makeNewVersionsHandler(db *postgres.DB, idxClient *index.Client, workers int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs, err := cron.FetchAndStoreVersions(r.Context(), idxClient, db)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("FetchAndStoreVersions(%q, db): %v", indexURL, err)
			return
		}
		log.Printf("Fetching %d versions", len(logs))

		client := fetch.New(fetchURL)
		cron.FetchVersions(r.Context(), client, logs, workers)
		fmt.Fprint(w, fmt.Sprintf("Requested %d new versions!", len(logs)))
	}
}

func makeRetryVersionsHandler(db *postgres.DB, workers int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs, err := db.GetVersionsToRetry(r.Context())
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("db.GetVersionsToRetry(ctx): %v", err)
			return
		}
		log.Printf("Fetching %d versions", len(logs))

		client := fetch.New(fetchURL)
		cron.FetchVersions(r.Context(), client, logs, workers)
		fmt.Fprint(w, fmt.Sprintf("Requested %d versions!", len(logs)))
	}
}

func main() {
	flag.Parse()

	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open(%q): %v", dbinfo, err)
	}
	defer db.Close()

	idxClient, err := index.New(indexURL)
	if err != nil {
		log.Fatalf("index.New(%q): %v", indexURL, err)
	}

	handlerTimeout, err := strconv.Atoi(timeout)
	if err != nil {
		log.Fatalf("strconv.Atoi(%q): %v", timeout, err)
	}
	mw := middleware.Timeout(time.Duration(handlerTimeout) * time.Minute)
	http.Handle("/new/", mw(makeNewVersionsHandler(db, idxClient, *workers)))
	http.Handle("/retry/", mw(makeRetryVersionsHandler(db, *workers)))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, Go Discovery Cron!")
	})

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

	log.Printf("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
