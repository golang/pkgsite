// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"strings"
)

// Result types combine base entity data with optional supplementary data.
// These are used for both text formatting and JSON output.

type packageResult struct {
	Package    *packageResponse                   `json:"package"`
	Symbols    *paginatedResponse[symbolResponse] `json:"symbols,omitempty"`
	ImportedBy *importedByResponse                `json:"importedBy,omitempty"`
}

type moduleResult struct {
	Module   *moduleResponse                           `json:"module"`
	Versions *paginatedResponse[versionResponse]       `json:"versions,omitempty"`
	Vulns    *paginatedResponse[vulnResponse]          `json:"vulns,omitempty"`
	Packages *paginatedResponse[modulePackageResponse] `json:"packages,omitempty"`
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

func formatModule(w io.Writer, r moduleResult) {
	m := r.Module
	fmt.Fprintf(w, "%s\n", m.Path)
	version := m.Version
	if m.IsLatest {
		version += " (latest)"
	}
	fmt.Fprintf(w, "  Version:          %s\n", version)
	if m.RepoURL != "" {
		fmt.Fprintf(w, "  Repository:       %s\n", m.RepoURL)
	}
	fmt.Fprintf(w, "  Has go.mod:       %s\n", yesNo(m.HasGoMod))
	fmt.Fprintf(w, "  Redistributable:  %s\n", yesNo(m.IsRedistributable))

	if m.Readme != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "README (%s):\n", m.Readme.Filepath)
		fmt.Fprintln(w, m.Readme.Contents)
	}

	if len(m.Licenses) > 0 {
		fmt.Fprintln(w)
		formatLicenses(w, m.Licenses)
	}

	if r.Versions != nil && len(r.Versions.Items) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Versions:")
		for _, v := range r.Versions.Items {
			fmt.Fprintf(w, "  %s\n", v.Version)
		}
		formatPaginationHint(w, len(r.Versions.Items), r.Versions.Total)
	}

	if r.Vulns != nil && len(r.Vulns.Items) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Vulnerabilities:")
		for _, v := range r.Vulns.Items {
			fmt.Fprintf(w, "  %s\n", v.ID)
			if v.Summary != "" {
				fmt.Fprintf(w, "    %s\n", v.Summary)
			} else if v.Details != "" {
				fmt.Fprintf(w, "    %s\n", firstLine(v.Details))
			}
			if v.FixedVersion != "" {
				fmt.Fprintf(w, "    Fixed in: %s\n", v.FixedVersion)
			}
		}
		formatPaginationHint(w, len(r.Vulns.Items), r.Vulns.Total)
	}

	if r.Packages != nil && len(r.Packages.Items) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Packages:")
		for _, p := range r.Packages.Items {
			if p.Synopsis != "" {
				fmt.Fprintf(w, "  %-40s %s\n", p.Path, p.Synopsis)
			} else {
				fmt.Fprintf(w, "  %s\n", p.Path)
			}
		}
		formatPaginationHint(w, len(r.Packages.Items), r.Packages.Total)
	}
}

func formatSearch(w io.Writer, r *paginatedResponse[searchResultResponse]) {
	if len(r.Items) == 0 {
		fmt.Fprintln(w, "No results.")
		return
	}
	for _, sr := range r.Items {
		fmt.Fprintf(w, "%s\n", sr.PackagePath)
		fmt.Fprintf(w, "  Module:   %s@%s\n", sr.ModulePath, sr.Version)
		if sr.Synopsis != "" {
			fmt.Fprintf(w, "  Synopsis: %s\n", sr.Synopsis)
		}
		fmt.Fprintln(w)
	}
	formatPaginationHint(w, len(r.Items), r.Total)
}

func formatLicenses(w io.Writer, licenses []licenseResponse) {
	fmt.Fprintln(w, "Licenses:")
	for _, l := range licenses {
		fmt.Fprintf(w, "  %s (%s)\n", strings.Join(l.Types, ", "), l.FilePath)
	}
}

func formatPaginationHint(w io.Writer, shown, total int) {
	// TODO(hyangah): show how to use token.
	if total > shown {
		fmt.Fprintf(w, "  Showing %d of %d. Use --limit=N to see more.\n", shown, total)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}
