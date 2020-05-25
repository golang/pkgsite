// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The teeproxy hits the frontend with a URL from godoc.org.
package main

import (
	"context"
	"net/http"
	"os"

	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/teeproxy"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	cfg.Dump(os.Stderr)
	if cfg.OnAppEngine() {
		_, err := log.UseStackdriver(ctx, cfg, "teeproxy-log")
		if err != nil {
			log.Fatal(ctx, err)
		}
	}

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "content/static/img/favicon.ico")
	})
	http.HandleFunc("/", teeproxy.HandleGddoEvent)

	addr := cfg.HostAddr("localhost:8020")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}
