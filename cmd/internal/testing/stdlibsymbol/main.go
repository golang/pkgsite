// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// stdlibsymbol compares data from the symbol_history table with
// the stdlib API data at
// https://go.googlesource.com/go/+/refs/heads/master/api.
package main

import (
	"context"
	"fmt"

	"golang.org/x/pkgsite/cmd/internal/cmdconfig"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/log"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Init(ctx)
	log.SetLevel("error")
	if err != nil {
		log.Fatal(ctx, err)
	}
	db, err := cmdconfig.OpenDB(ctx, cfg, false)
	if err != nil {
		log.Fatal(ctx, err)
	}

	pkgToErrors, err := db.CompareStdLib(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	for path, errs := range pkgToErrors {
		fmt.Printf("----- %s -----\n", path)
		for _, e := range errs {
			fmt.Print(e)
		}
	}
}
