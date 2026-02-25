// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// Package represents the response for /v1/package/{packagePath}.
type Package struct {
	Path              string    `json:"path"`
	ModulePath        string    `json:"modulePath"`
	ModuleVersion     string    `json:"moduleVersion"`
	Synopsis          string    `json:"synopsis"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	IsLatest          bool      `json:"isLatest"`
	GOOS              string    `json:"goos"`
	GOARCH            string    `json:"goarch"`
	Docs              string    `json:"docs,omitempty"`
	Imports           []string  `json:"imports,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// License represents license information in API responses.
type License struct {
	Types    []string `json:"types"`
	FilePath string   `json:"filePath"`
	Contents string   `json:"contents,omitempty"`
}

// PaginatedResponse is a generic paginated response.
type PaginatedResponse[T any] struct {
	Items         []T    `json:"items"`
	Total         int    `json:"total"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// PackageImportedBy represents the response for /v1/imported-by/{packagePath}.
type PackageImportedBy struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	ImportedBy PaginatedResponse[string] `json:"importedBy"`
}

// Module represents the response for /v1/module/{modulePath}.
type Module struct {
	Path              string    `json:"path"`
	Version           string    `json:"version"`
	IsLatest          bool      `json:"isLatest"`
	IsRedistributable bool      `json:"isRedistributable"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	RepoURL           string    `json:"repoUrl"`
	GoModContents     string    `json:"goModContents,omitempty"`
	Readme            *Readme   `json:"readme,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// Readme represents a readme file.
type Readme struct {
	Filepath string `json:"filepath"`
	Contents string `json:"contents"`
}

// Symbol represents a symbol in /v1/symbols/{packagePath}.
type Symbol struct {
	ModulePath string `json:"modulePath"`
	Version    string `json:"version"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Synopsis   string `json:"synopsis"`
	Parent     string `json:"parent,omitempty"`
}

// SearchResults represents the response for /v1/search?q={query}.
type SearchResult struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

// Vulnerability represents a vulnerability in /v1/vulnerabilities/{modulePath}.
type Vulnerability struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}
