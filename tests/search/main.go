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
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
)

var frontendHost = flag.String("frontend", "http://localhost:8080",
	"Use the frontend host referred to by this URL for comparing data")

func main() {
	flag.Parse()

	ctx := context.Background()
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	log.SetLevel(cfg.LogLevel)

	if err := runImportedByUpdates(ctx, cfg.DBConnInfo(), cfg.DBHost); err != nil {
		log.Fatal(ctx, err)
	}
	if err := run(*frontendHost); err != nil {
		log.Fatal(ctx, err)
	}
}

const (
	importedbyFile = "tests/search/importedby.txt"
)

var testFiles = []string{
	"tests/search/scripts/default.txt",
	"tests/search/scripts/symbolsearch.txt",
}

func runImportedByUpdates(ctx context.Context, dbConnInfo, dbHost string) error {
	ddb, err := database.Open("pgx", dbConnInfo, "seeddb")
	if err != nil {
		log.Fatalf(ctx, "database.Open for host %s failed with %v", dbHost, err)
	}
	db := postgres.New(ddb)
	defer db.Close()
	counts, err := readImportedByCounts(importedbyFile)
	if err != nil {
		return err
	}
	_, err = db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, counts)
	return err
}

func run(frontendHost string) error {
	var tests []*searchTest
	for _, testFile := range testFiles {
		ts, err := readSearchTests(testFile)
		if err != nil {
			return err
		}
		tests = append(tests, ts...)
	}
	client := frontend.NewClient(frontendHost)
	var failed bool
	for _, st := range tests {
		output, err := runTest(client, st)
		if err != nil {
			return err
		}
		if len(output) == 0 {
			fmt.Println("--- PASSED: ", st.title)
			continue
		}
		failed = true
		fmt.Println("--- FAILED: ", st.title)
		for _, e := range output {
			fmt.Println(e)
		}
	}
	if failed {
		return fmt.Errorf("SEARCH TESTS FAILED: see output above")
	}
	return nil
}

func runTest(client *frontend.Client, st *searchTest) (output []string, err error) {
	defer derrors.Wrap(&err, "runTest(ctx, db, st.title: %q)", st.title)
	searchPage, err := client.Search(st.query, st.mode)
	if err != nil {
		return nil, err
	}
	gotResults := searchPage.Results
	if strings.ContainsAny(searchPage.PackageTabQuery, "#") {
		output = append(output, "invalid package tab query, should not contain #: %q", searchPage.PackageTabQuery)
	}
	for i, want := range st.results {
		got := &frontend.SearchResult{}
		if len(gotResults) > i {
			got = gotResults[i]
		}
		// The mode we expect is determined by whether the expected result
		// indicates a symbol is present.
		wantMode := "package"
		if want.symbol != "" {
			wantMode = "symbol"
		}
		if want.symbol != got.SymbolName || want.pkg != got.PackagePath || wantMode != searchPage.SearchMode {
			output = append(output,
				fmt.Sprintf("query: %q, mismatch result %d:\n\twant: %q %q [m=%q]\n\t got: %q %q [m=%q]\n",
					st.query, i+1,
					want.pkg, want.symbol, wantMode,
					got.PackagePath, got.SymbolName, searchPage.SearchMode))
		}
	}
	return output, nil
}

type searchTest struct {
	title   string
	query   string
	mode    string
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
				return nil, fmt.Errorf("invalid syntax on line %d (%q): %q", num, filename, line)
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
				parts := strings.Fields(line)
				mode := strings.TrimSuffix(strings.TrimPrefix(parts[0], "["), "]")
				if len(parts) <= 1 {
					return nil, fmt.Errorf("invalid syntax on line %d: %q (not enough elements)", num, line)
				}
				if mode != "" && mode != "package" && mode != "symbol" {
					return nil, fmt.Errorf("invalid syntax on line %d: %q (invalid mode: %q)", num, line, mode)
				}
				test.mode = mode
				test.query = strings.Join(parts[1:], " ")
			case posQuery, posResult:
				// The last position was a query or a result, so this must be
				// an expected search result.
				curr = posResult
				parts := strings.Split(line, " ")
				r := &searchResult{}
				if test.mode == "symbol" {
					if len(parts) != 2 {
						return nil, fmt.Errorf("invalid syntax on line %d (%q): %q (want symbol result)", num, filename, line)
					}
					r.symbol = parts[0]
					r.pkg = parts[1]
				} else {
					if len(parts) != 1 {
						return nil, fmt.Errorf("invalid syntax on line %d (%q): %q (want package result)", num, filename, line)
					}
					r.pkg = parts[0]
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
		path, count, found := strings.Cut(line, ", ")
		if !found {
			return nil, errors.New("missing comma")
		}
		c, err := strconv.Atoi(count)
		if err != nil {
			return nil, err
		}
		counts[path] = c
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
