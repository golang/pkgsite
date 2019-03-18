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

	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

var (
	proxyURL = getEnv("GO_MODULE_PROXY_URL", "")
	user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	dbinfo   = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, dbname)
	port     = getEnv("PORT", "9000")
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func makeFetchHandler(proxyClient *proxy.Client, db *postgres.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprintf(w, "Hello, Go Discovery Fetch Service!")
			return
		}

		log.Printf("Request received: %q", r.URL.Path)
		module, version, err := fetch.ParseModulePathAndVersion(r.URL)
		if err != nil {
			http.Error(w, fmt.Sprintf("Status %d (%s): %v", http.StatusBadRequest, http.StatusText(http.StatusBadRequest), err),
				http.StatusBadRequest)
			return
		}

		if err := fetch.FetchAndInsertVersion(module, version, proxyClient, db); err != nil {
			http.Error(w, fmt.Sprintf("Status %d (%s): %v", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), err),
				http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "Downloaded: %q %q", module, version)
	}
}

func main() {
	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/", makeFetchHandler(proxy.New(proxyURL), db))

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
