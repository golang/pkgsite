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
	if err := ioutil.WriteFile(filename, []byte(contents), 0644); err != nil {
		return fmt.Errorf("ioutil.WriteFile(f, '', 0644): %v", err)
	}
	return nil
}

var contents = fmt.Sprintf(`// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated with go generate -run gen_query.go. DO NOT EDIT.

package symbolsearch

// QuerySymbol is used when the search query is only one word, with no dots.
// In this case, the word must match a symbol name and ranking is completely
// determined by the path_tokens.
%s`, formatQuery("QuerySymbol", symbolsearch.RawQuerySymbol))

func formatQuery(name, query string) string {
	return fmt.Sprintf("const %s = `%s`", name, query)
}
