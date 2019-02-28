// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The fetch command runs a server that fetches modules from a proxy and writes
// them to the discovery database.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"

	"golang.org/x/discovery/internal/cron"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/postgres"
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
		logs, err := cron.NewVersionsFromProxyIndex(indexURL, db)
		if err != nil {
			http.Error(w, fmt.Sprintf("Status %d (%s): %v", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err),
				http.StatusInternalServerError)
			return
		}

		client := fetch.New(fetchURL)
		for _, l := range logs {
			fmt.Fprintf(w, "Fetch requested: %q %q\n", l.Name, l.Version)
			go func(name, version string) {
				if err := client.FetchVersion(name, version); err != nil {
					log.Printf("client.FetchVersion(%q, %q): %v", name, version, err)
				}
			}(l.Name, l.Version)
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

	http.HandleFunc("/new/", makeNewVersionsHandler(db))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, Go Discovery Cron!")
	})

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
