// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"strings"
)

type packageResult struct {
	Package    *packageResponse                   `json:"package"`
	Symbols    *paginatedResponse[symbolResponse] `json:"symbols,omitempty"`
	ImportedBy *importedByResponse                `json:"importedBy,omitempty"`
}

func formatPackage(w io.Writer, r packageResult) {
	p := r.Package
	if p.IsStandardLibrary {
		fmt.Fprintf(w, "%s (standard library)\n", p.Path)
	} else {
		fmt.Fprintf(w, "%s\n", p.Path)
	}
	fmt.Fprintf(w, "  Module:   %s\n", p.ModulePath)
	version := p.ModuleVersion
	if p.IsLatest {
		version += " (latest)"
	}
	fmt.Fprintf(w, "  Version:  %s\n", version)
	if p.Synopsis != "" {
		fmt.Fprintf(w, "  Synopsis: %s\n", p.Synopsis)
	}

	if p.Docs != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, p.Docs)
	}

	if len(p.Imports) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Imports:")
		for _, imp := range p.Imports {
			fmt.Fprintf(w, "  %s\n", imp)
		}
	}

	if len(p.Licenses) > 0 {
		fmt.Fprintln(w)
		formatLicenses(w, p.Licenses)
	}

	if r.Symbols != nil && len(r.Symbols.Items) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Symbols:")
		for _, s := range r.Symbols.Items {
			if s.Synopsis != "" {
				fmt.Fprintf(w, "  %s\n", s.Synopsis)
			} else {
				fmt.Fprintf(w, "  %s %s\n", s.Kind, s.Name)
			}
		}
		formatPaginationHint(w, len(r.Symbols.Items), r.Symbols.Total)
	}

	if r.ImportedBy != nil {
		ib := r.ImportedBy.ImportedBy
		if len(ib.Items) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Imported by:")
			for _, pkg := range ib.Items {
				fmt.Fprintf(w, "  %s\n", pkg)
			}
			formatPaginationHint(w, len(ib.Items), ib.Total)
		}
	}
}

func formatLicenses(w io.Writer, licenses []licenseResponse) {
	fmt.Fprintln(w, "Licenses:")
	for _, l := range licenses {
		fmt.Fprintf(w, "  %s (%s)\n", strings.Join(l.Types, ", "), l.FilePath)
	}
}

func formatPaginationHint(w io.Writer, shown, total int) {
	if total > shown {
		fmt.Fprintf(w, "  Showing %d of %d. Use --limit=N to see more.\n", shown, total)
	}
}
