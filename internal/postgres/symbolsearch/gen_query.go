// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"go/format"
	"io/ioutil"

	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres/symbolsearch"
)

const filename = "query.gen.go"

func main() {
	ctx := context.Background()
	if err := generateFile(ctx, filename); err != nil {
		log.Fatal(ctx, err)
	}
	fmt.Printf("Wrote %s.\n", filename)
}

// generateFile writes symbol search queries to filename.
func generateFile(ctx context.Context, filename string) error {
	content, err := format.Source([]byte(symbolsearch.Content))
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("ioutil.WriteFile(f, '', 0644): %v", err)
	}
	return nil
}
