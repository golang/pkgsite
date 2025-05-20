// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// doc is an internal command started by go doc -http
// to serve documentation. It should not be invoked
// directly.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"golang.org/x/pkgsite/cmd/internal/pkgsite"
	"golang.org/x/pkgsite/internal/browser"
	ilog "golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware/timeout"
	"golang.org/x/pkgsite/internal/stdlib"
)

var (
	goRepoPath = flag.String("gorepo", "", "")
	addr       = flag.String("http", "", "")
	pathToOpen = flag.String("open", "", "")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("doc: ")

	// Print simple log entries without the severity, and
	// print error-level and above log messages to the user
	ilog.Use(docLogger{})
	ilog.SetLevel("error")

	flag.Parse()
	if *goRepoPath == "" || *addr == "" || *pathToOpen == "" {
		log.Fatal("-gorepo, -http, or -open not provided to doc command")
	}

	stdlib.SetGoRepoPath(*goRepoPath)

	ctx := context.Background()
	server, err := pkgsite.BuildServer(ctx, pkgsite.ServerConfig{
		AllowNoModules: true,
		UseListedMods:  true,
		UseLocalStdlib: true,
		GoRepoPath:     *goRepoPath,
	})
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	url := "http://" + *addr
	log.Printf("Documentation server listening on addr %s", url)

	go func() {
		if !browser.Open(*pathToOpen) {
			log.Printf("Failed to open browser window. Please visit %s in your browser.", *pathToOpen)
		}
	}()

	router := http.NewServeMux()
	server.Install(router.Handle, nil, nil)
	mw := timeout.Timeout(54 * time.Second)
	srv := &http.Server{Addr: *addr, Handler: mw(router)}
	log.Fatal(srv.Serve(ln))
}

// docLogger is a simple logger that prints the payload
// using the standard library log package.
type docLogger struct{}

func (docLogger) Log(ctx context.Context, s ilog.Severity, payload any) { log.Printf("%+v", payload) }
func (docLogger) Flush()                                                {}
