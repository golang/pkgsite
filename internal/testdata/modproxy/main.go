// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"net/http"
)

const proxyURL = "./proxy"

func main() {
	http.Handle("/", http.FileServer(http.Dir(proxyURL)))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
