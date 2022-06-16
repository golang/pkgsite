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
	"os"

	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres/search"
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
	content, err := format.Source([]byte(search.Content))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("os.WriteFile(f, '', 0644): %v", err)
	}
	return nil
}
