// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"golang.org/x/pkgsite/cmd/internal/pkgsite-cli/client"
)

func runPackage(fs *flag.FlagSet, p *packageFlags, stdout, stderr io.Writer) int {
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "Error: expected exactly 1 package argument, got %d\n", fs.NArg())
		fs.Usage()
		return 2
	}
	path, version := splitPathVersion(fs.Arg(0))

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	c, err := client.New(p.server)
	if err != nil {
		handleErr(stdout, stderr, err, p.jsonOut)
		return 1
	}
	c.PrintURLs = p.printURLs
	c.Output = stderr
	pkg, err := c.GetPackage(ctx, path, version, client.PackageOptions{
		Module:   p.module,
		Doc:      p.doc,
		Examples: p.examples,
		Imports:  p.imports,
		Licenses: p.licenses,
		GOOS:     p.goos,
		GOARCH:   p.goarch,
	})
	if err != nil {
		handleErr(stdout, stderr, err, p.jsonOut)
		return 1
	}
	result := packageResult{Package: pkg}

	if p.symbols {
		fetch := func(token string, limit int) (*client.PaginatedResponse[client.Symbol], error) {
			return c.GetSymbols(ctx, path, version, client.SymbolsOptions{
				Module: p.module,
				GOOS:   p.goos,
				GOARCH: p.goarch,
				PaginationOptions: client.PaginationOptions{
					Limit: limit,
					Token: token,
				},
			})
		}
		items, total, nextToken, err := client.AllItems(p.symbolToken, p.effectiveLimit(), fetch)
		if err != nil {
			if client.Is429(err) {
				result.Symbols = &client.PaginatedResponse[client.Symbol]{
					Items:         items,
					Total:         total,
					NextPageToken: nextToken,
				}
				return printPartialPackageResult(stdout, stderr, result, p.jsonOut)
			}
			handleErr(stdout, stderr, err, p.jsonOut)
			return 1
		}
		result.Symbols = &client.PaginatedResponse[client.Symbol]{
			Items: items,
			Total: total,
		}
	}
	if p.importedBy {
		var initialResp *client.PackageImportedBy
		fetch := func(token string, limit int) (*client.PaginatedResponse[string], error) {
			r, err := c.GetImportedBy(ctx, path, version, client.ImportedByOptions{
				Module: p.module,
				PaginationOptions: client.PaginationOptions{
					Limit: limit,
					Token: token,
				},
			})
			if err != nil {
				return nil, err
			}
			if initialResp == nil {
				initialResp = r
			}
			return &r.ImportedBy, nil
		}

		items, total, nextToken, err := client.AllItems(p.importedByToken, p.effectiveLimit(), fetch)
		if err != nil {
			if client.Is429(err) {
				if initialResp == nil {
					initialResp = &client.PackageImportedBy{}
				}
				result.ImportedBy = initialResp
				result.ImportedBy.ImportedBy.Items = items
				result.ImportedBy.ImportedBy.Total = total
				result.ImportedBy.ImportedBy.NextPageToken = nextToken
				return printPartialPackageResult(stdout, stderr, result, p.jsonOut)
			}
			handleErr(stdout, stderr, err, p.jsonOut)
			return 1
		}

		result.ImportedBy = initialResp
		result.ImportedBy.ImportedBy.Items = items
		result.ImportedBy.ImportedBy.Total = total
	}

	if p.jsonOut {
		return writeJSON(stdout, stderr, result)
	}
	formatPackage(stdout, result)
	return 0
}

// packageFlags are flags for the package subcommand.
type packageFlags struct {
	commonFlags
	doc             string
	examples        bool
	imports         bool
	importedBy      bool
	symbols         bool
	licenses        bool
	module          string
	goos            string
	goarch          string
	symbolToken     string
	importedByToken string
}

func (f *packageFlags) register(fs *flag.FlagSet) {
	f.commonFlags.register(fs)
	fs.StringVar(&f.doc, "doc", "", "render docs in format: text, md, html")
	fs.BoolVar(&f.examples, "examples", false, "include examples (requires -doc)")
	fs.BoolVar(&f.imports, "imports", false, "list imported packages")
	fs.BoolVar(&f.importedBy, "imported-by", false, "list reverse dependencies")
	fs.BoolVar(&f.symbols, "symbols", false, "list exported symbols")
	fs.BoolVar(&f.licenses, "licenses", false, "show license information")
	fs.StringVar(&f.module, "module", "", "disambiguate module path")
	fs.StringVar(&f.goos, "goos", "", "target GOOS")
	fs.StringVar(&f.goarch, "goarch", "", "target GOARCH")
	fs.StringVar(&f.symbolToken, "symbol-token", "", "page token for symbols pagination")
	fs.StringVar(&f.importedByToken, "imported-by-token", "", "page token for imported-by pagination")
}

func printPartialPackageResult(stdout, stderr io.Writer, result packageResult, jsonMode bool) int {
	if jsonMode {
		writeJSON(stdout, stderr, result)
	} else {
		formatPackage(stdout, result)
		fmt.Fprintln(stderr, "Warning: hit rate limit (429); results are incomplete.")
	}
	return 1
}
