// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The search command is used to run tests for search using a dataset of
// modules specified at tests/search/seed.txt.
// See tests/README.md for details.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
)

func main() {
	flag.Parse()

	ctx := context.Background()
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
		internal.ExperimentSearchGrouping,
		internal.ExperimentSymbolSearch)
	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.SetLevel(cfg.LogLevel)

	// Wrap the postgres driver with our own wrapper, which adds OpenCensus instrumentation.
	ddb, err := database.Open("pgx", cfg.DBConnInfo(), "seeddb")
	if err != nil {
		log.Fatalf(ctx, "database.Open for host %s failed with %v", cfg.DBHost, err)
	}
	db := postgres.New(ddb)
	defer db.Close()

	if err := run(ctx, db); err != nil {
		log.Fatal(ctx, err)
	}
}

func run(ctx context.Context, db *postgres.DB) error {
	counts, err := readImportedByCounts("tests/search/importedby.txt")
	if err != nil {
		return err
	}
	if _, err := db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, counts); err != nil {
		return err
	}

	tests, err := readSearchTests("tests/search/scripts/symbolsearch.txt")
	if err != nil {
		return err
	}
	ctx = experiment.NewContext(ctx,
		internal.ExperimentInsertSymbolSearchDocuments,
		internal.ExperimentSearchGrouping,
		internal.ExperimentSymbolSearch)
	for _, st := range tests {
		results, err := db.Search(ctx, st.query, postgres.SearchOptions{MaxResults: 10, SearchSymbols: true})
		if err != nil {
			return err
		}
		var errors []string
		for i, want := range st.results {
			got := &postgres.SearchResult{}
			if len(results) > i {
				got = results[i]
			}
			if want.symbol != got.SymbolName || want.pkg != got.PackagePath {
				errors = append(errors,
					fmt.Sprintf("query %s, mismatch result %d:\n\twant: %q %q\n\t got: %q %q\n",
						st.query, i+1,
						want.pkg, want.symbol,
						got.PackagePath, got.SymbolName))
			}
		}
		if len(errors) > 0 {
			fmt.Println("--- FAILED: ", st.title)
			for _, e := range errors {
				fmt.Println(e)
			}
		} else {
			fmt.Println("--- PASSED: ", st.title)
		}
	}
	return nil
}

type searchTest struct {
	title   string
	query   string
	results []*searchResult
}

type searchResult struct {
	pkg    string
	symbol string
}

// readSearchTests reads filename and returns the search tests from that file.
// See tests/README.md for a description of the syntax.
func readSearchTests(filename string) ([]*searchTest, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)

	var (
		tests []*searchTest
		test  searchTest
		num   int
		curr  = posNewline
	)
	for scan.Scan() {
		num += 1
		line := strings.TrimSpace(scan.Text())

		var prefix string
		if len(line) > 0 {
			prefix = string(line[0])
		}
		switch prefix {
		case "#":
			// Skip comment lines.
			continue
		case "":
			// Each set of tests is separated by a newline. Before a newline, we must
			// have passed a test case result, another newline, or a comment,
			// otherwise this file can't be valid.
			if curr != posNewline && curr != posResult {
				return nil, fmt.Errorf("invalid syntax on line %d: %q", num, line)
			}
			if curr == posResult {
				// This is the first time that we have seen a newline for this
				// test set. Now that we know the test set is complete, append
				// it to the array of tests, and reset test to an empty
				// searchTest struct.
				t2 := test
				tests = append(tests, &t2)
				test = searchTest{}
			}
			curr = posNewline
		default:
			switch curr {
			case posNewline:
				// The last position was a newline, so this must be the start
				// of a new test set.
				curr = posTitle
				test.title = line
			case posTitle:
				// The last position was a title, so this must be the start
				// of a new test set.
				curr = posQuery
				test.query = line
			case posQuery, posResult:
				// The last position was a query or a result, so this must be
				// an expected search result.
				curr = posResult
				parts := strings.Split(line, " ")
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid syntax on line %d: %q", num, line)
				}
				r := &searchResult{
					symbol: parts[0],
					pkg:    parts[1],
				}
				test.results = append(test.results, r)
			default:
				// We should never reach this error.
				return nil, fmt.Errorf("invalid syntax on line %d: %q", num, line)
			}
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("scan.Err(): %v", err)
	}
	tests = append(tests, &test)
	return tests, nil
}

// readSearchTests reads filename and returns a map of package path to imported
// by count. See tests/README.md for a description of the syntax.
func readImportedByCounts(filename string) (map[string]int, error) {
	counts := map[string]int{}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, ", ", 2)
		c, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		counts[parts[0]] = c
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

const (
	posNewline = 1 << iota
	posTitle
	posQuery
	posResult
)
