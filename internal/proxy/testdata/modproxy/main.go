// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// modproxy runs a local module proxy for testing. It implements the Module
// proxy protocol described at `go help goproxy` by serving files stored at
// ./proxy. The following modules are supported by this proxy:
// my.mod/module   v1.0.0
// my.mod/module   v1.1.0
// my.mod/module/2 v12.0.0
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"golang.org/x/discovery/internal/proxy"
)

var proxyURL, _ = filepath.Abs("proxy")

func main() {
	http.Handle("/", proxy.TestProxy(nil))

	addr := ":7000"
	log.Println(fmt.Sprintf("Listening on http://localhost%s", addr))
	log.Fatal(http.ListenAndServe(addr, nil))
}
