// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The teeproxy hits the frontend with a URL from godoc.org.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"golang.org/x/pkgsite/internal/breaker"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/dcensus"
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

	views := append(dcensus.ServerViews,
		teeproxy.TeeproxyGddoRequestCount,
		teeproxy.TeeproxyPkgGoDevRequestCount,
		teeproxy.TeeproxyGddoRequestLatencyDistribution,
		teeproxy.TeeproxyPkgGoDevRequestLatencyDistribution,
	)
	dcensus.Init(cfg, views...)

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "content/static/img/favicon.ico")
	})
	server, err := teeproxy.NewServer(teeproxy.Config{
		Rate:  20,
		Burst: 20,
		BreakerConfig: breaker.Config{
			FailsToRed:       10,
			FailureThreshold: 0.5,
			GreenInterval:    10 * time.Second,
			MinTimeout:       30 * time.Second,
			MaxTimeout:       4 * time.Minute,
			SuccsToGreen:     20,
		},
		ShouldForward: cfg.TeexproxyForwarding,
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	http.Handle("/", server)

	addr := cfg.HostAddr("localhost:8020")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}
