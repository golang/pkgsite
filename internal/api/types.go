// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Package is the response for /v1beta/package/{packagePath}.
type Package struct {
	ModulePath        string    `json:"modulePath"`
	Version           string    `json:"version"`
	IsLatest          bool      `json:"isLatest"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	GOOS              string    `json:"goos"`
	GOARCH            string    `json:"goarch"`
	Docs              string    `json:"docs,omitempty"`
	Imports           []string  `json:"imports,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
	PackageInfo
}

type PackageInfo struct {
	Path              string `json:"path"`
	Name              string `json:"name"`
	Synopsis          string `json:"synopsis"`
	IsRedistributable bool   `json:"isRedistributable"` // Whether the license allows distribution.
}

type PackagesResponse struct {
	ModulePath        string                         `json:"modulePath"`
	Version           string                         `json:"version"`
	IsStandardLibrary bool                           `json:"isStandardLibrary"`
	Packages          PaginatedResponse[PackageInfo] `json:"packages"`
}

// License is license information in API responses.
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

// PackageImportedBy is the response for /v1beta/imported-by/{packagePath}.
type PackageImportedBy struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	ImportedBy PaginatedResponse[string] `json:"importedBy"`
}

// ModuleVersion is the response for /v1beta/versions/{path}.
type ModuleVersion struct {
	ModulePath        string    `json:"modulePath"`
	Version           string    `json:"version"`
	CommitTime        time.Time `json:"commitTime"`
	IsRedistributable bool      `json:"isRedistributable"` // Whether the license allows distribution.
	HasGoMod          bool      `json:"hasGoMod"`          // Whether the module has a go.mod file.
	LatestVersion     string    `json:"latestVersion"`     // latest unretracted version
	Deprecated        bool      `json:"deprecated"`
	DeprecationReason string    `json:"deprecationReason"`
	Retracted         bool      `json:"retracted"`
	RetractionReason  string    `json:"retractionReason"`
}

// Module is the response for /v1beta/module/{modulePath}.
type Module struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// CommitTime is the timestamp returned by the module proxy's .info endpoint,
	// representing the time the version was created.
	CommitTime        time.Time `json:"commitTime"`
	IsLatest          bool      `json:"isLatest"`
	IsRedistributable bool      `json:"isRedistributable"`
	IsStandardLibrary bool      `json:"isStandardLibrary"`
	HasGoMod          bool      `json:"hasGoMod"`
	RepoURL           string    `json:"repoUrl"`
	GoModContents     string    `json:"goModContents,omitempty"`
	Readme            *Readme   `json:"readme,omitempty"`
	Licenses          []License `json:"licenses,omitempty"`
}

// Readme is a readme file.
type Readme struct {
	Filepath string `json:"filepath"`
	Contents string `json:"contents"`
}

// PackageSymbols is the response for /v1beta/symbols/{packagePath}.
type PackageSymbols struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	Symbols    PaginatedResponse[Symbol] `json:"symbols"`
}

// Symbol is a symbol in a package.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Synopsis string `json:"synopsis"`
	Parent   string `json:"parent,omitempty"`
}

// SearchResults is the response for /v1beta/search?q={query}.
type SearchResult struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

// Vulnerability is a vulnerability in /v1beta/vulnerabilities/{modulePath}.
type Vulnerability struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}

// Error contains detailed information about an error.
type Error struct {
	Code       int         `json:"code"` // HTTP status code
	Message    string      `json:"message"`
	Fixes      []string    `json:"fixes"` // suggestions for how to fix
	Candidates []Candidate `json:"candidates,omitempty"`

	err error // Unexported field for internal tracking
}

func (e *Error) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.err
}

// BadRequest returns an Error with StatusBadRequest.
func BadRequest(msg string, fixes ...string) *Error {
	return &Error{
		Code:    http.StatusBadRequest,
		Message: msg,
		Fixes:   fixes,
	}
}

// InternalServerError returns an Error with StatusInternalServerError.
func InternalServerError(format string, args ...any) *Error {
	return &Error{
		Code:    http.StatusInternalServerError,
		Fixes:   []string{"File a bug at go.dev/issues"},
		Message: strings.ToLower(http.StatusText(http.StatusInternalServerError)),
		err:     fmt.Errorf(format, args...),
	}
}

// A Candidate is a potential resolution for an ambiguous path.
type Candidate struct {
	ModulePath  string `json:"modulePath"`
	PackagePath string `json:"packagePath"`
}
