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
	proxyURL = getEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org")
	user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	dbinfo   = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, dbname)
	addr     = getEnv("GO_DISCOVERY_FETCH_ADDR", "localhost:9000")
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
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<h1>Hello, Go Discovery Fetch Service!</h1>")
			fmt.Fprintf(w, `<p><a href="/rsc.io/quote/@v/v1.0.0">Fetch an example module</a></p>`)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			return
		}

		module, version, err := fetch.ParseModulePathAndVersion(r.URL.Path)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			log.Printf("ParseModulePathAndVersion(%q): %v", r.URL.Path, err)
			return
		}

		if err := fetch.FetchAndInsertVersion(module, version, proxyClient, db); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			log.Printf("FetchAndInsertVersion(%q, %q, proxyClient, db): %v", module, version, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "Downloaded module")
		log.Printf("Downloaded: %q %q", module, version)
	}
}

func main() {
	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/", makeFetchHandler(proxy.New(proxyURL), db))

	log.Printf("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
