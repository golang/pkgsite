// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"golang.org/x/pkgsite/cmd/internal/pkgsite-cli/client"
)

func runSearch(fs *flag.FlagSet, s *searchFlags, stdout, stderr io.Writer) int {
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "Error: expected at least 1 search query argument")
		fs.Usage()
		return 2
	}
	query := strings.Join(fs.Args(), " ")

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	c, err := client.New(s.server)
	if err != nil {
		handleErr(stdout, stderr, err, s.jsonOut)
		return 1
	}
	c.PrintURLs = s.printURLs
	c.Output = stderr

	fetch := func(token string, limit int) (*client.PaginatedResponse[client.SearchResult], error) {
		return c.Search(ctx, query, client.SearchOptions{
			Symbol: s.symbol,
			PaginationOptions: client.PaginationOptions{
				Limit: limit,
				Token: token,
			},
		})
	}

	var results *client.PaginatedResponse[client.SearchResult]

	targetLimit := s.effectiveLimit()

	items, total, nextToken, err := client.AllItems(s.token, targetLimit, fetch)
	if err != nil {
		if client.Is429(err) {
			results = &client.PaginatedResponse[client.SearchResult]{
				Items:         items,
				Total:         total,
				NextPageToken: nextToken,
			}
			if s.jsonOut {
				writeJSON(stdout, stderr, results)
			} else {
				formatSearch(stdout, results)
				fmt.Fprintln(stderr, "Warning: hit rate limit (429); results are incomplete.")
			}
			return 1
		}
		handleErr(stdout, stderr, err, s.jsonOut)
		return 1
	}
	results = &client.PaginatedResponse[client.SearchResult]{
		Items: items,
		Total: total,
	}

	if s.jsonOut {
		return writeJSON(stdout, stderr, results)
	}
	formatSearch(stdout, results)
	return 0
}

// searchFlags are flags for the search subcommand.
type searchFlags struct {
	commonFlags
	symbol string
	token  string
}

func (f *searchFlags) register(fs *flag.FlagSet) {
	f.commonFlags.register(fs)
	fs.StringVar(&f.symbol, "symbol", "", "search for a symbol")
	fs.StringVar(&f.token, "token", "", "page token for pagination")
}
