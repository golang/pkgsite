// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The teeproxy hits the frontend with a URL from godoc.org.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"golang.org/x/pkgsite/internal/auth"
	"golang.org/x/pkgsite/internal/breaker"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/secrets"
	"golang.org/x/pkgsite/internal/teeproxy"
)

func main() {
	var credsFile = flag.String("creds", "", "filename for credentials, when running locally")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
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
	var client *http.Client
	var jsonCreds []byte
	if *credsFile != "" {
		jsonCreds, err = ioutil.ReadFile(*credsFile)
		if err != nil {
			log.Fatal(ctx, err)
		}
	} else {
		const secretName = "load-test-agent-creds"
		log.Infof(ctx, "getting secret %q", secretName)
		s, err := secrets.Get(context.Background(), secretName)
		if err != nil {
			log.Fatal(ctx, err)
		}
		jsonCreds = []byte(s)
	}
	client, err = auth.NewClient(jsonCreds)
	if err != nil {
		log.Fatal(ctx, err)
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
		AuthKey:   cfg.AuthHeader,
		AuthValue: cfg.TeeproxyAuthValue,
		Rate:      20,
		Burst:     20,
		BreakerConfig: breaker.Config{
			FailsToRed:       10,
			FailureThreshold: 0.5,
			GreenInterval:    10 * time.Second,
			MinTimeout:       30 * time.Second,
			MaxTimeout:       4 * time.Minute,
			SuccsToGreen:     20,
		},
		Hosts:  cfg.TeeproxyForwardedHosts,
		Client: client,
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	http.Handle("/", server)

	addr := cfg.HostAddr("localhost:8020")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}
