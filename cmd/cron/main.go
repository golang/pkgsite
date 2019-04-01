// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"golang.org/x/discovery/internal/cron"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

const (
	// Use generous timeouts as cron traffic is not user-facing.
	makeNewVersionsTimeout = 10 * time.Minute
	fetchTimeout           = 5 * time.Minute
)

var (
	indexURL = getEnv("GO_MODULE_INDEX_URL", "")
	fetchURL = getEnv("GO_DISCOVERY_FETCH_URL", "http://localhost:9000")
	user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	dbinfo   = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, dbname)
	port     = getEnv("PORT", "8000")
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func makeNewVersionsHandler(db *postgres.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs, err := cron.FetchAndStoreVersions(r.Context(), indexURL, db)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("FetchAndStoreVersions(%q, db): %v", indexURL, db)
			return
		}

		client := fetch.New(fetchURL)
		for _, l := range logs {
			fmt.Fprintln(w, "Fetch requested")
			log.Printf("Fetch requested: %q %q", l.ModulePath, l.Version)
			go func(name, version string) {
				fetchCtx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
				defer cancel()
				if err := client.FetchVersion(fetchCtx, name, version); err != nil {
					log.Printf("client.FetchVersion(fetchCtx, %q, %q): %v", name, version, err)
				}
			}(l.ModulePath, l.Version)
		}
		fmt.Fprintf(w, "Done!")
	}
}

func main() {
	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open(%q): %v", dbinfo, err)
	}
	defer db.Close()

	mw := middleware.Timeout(makeNewVersionsTimeout)
	http.Handle("/new/", mw(makeNewVersionsHandler(db)))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, Go Discovery Cron!")
	})

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
