// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

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
	rows := []string{
		header,
		`package symbolsearch`,
		symbolsearch.ConstructQuerySymbol(),
	}
	var contents []string
	for i, row := range rows {
		contents = append(contents, row)
		if i != (len(rows) - 1) {
			contents = append(contents, "\n\n")
		}
	}

	if err := ioutil.WriteFile(filename, []byte(strings.Join(contents, "")), 0644); err != nil {
		return fmt.Errorf("ioutil.WriteFile(f, '', 0644): %v", err)
	}
	return nil
}

const header = `// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated with go generate -run gen_query.go. DO NOT EDIT.`
