// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strings"
)

// Package is the response for /v1/package/{packagePath}.
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

// PackageImportedBy is the response for /v1/imported-by/{packagePath}.
type PackageImportedBy struct {
	ModulePath string                    `json:"modulePath"`
	Version    string                    `json:"version"`
	ImportedBy PaginatedResponse[string] `json:"importedBy"`
}

// Module is the response for /v1/module/{modulePath}.
type Module struct {
	Path              string    `json:"path"`
	Version           string    `json:"version"`
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

// Symbol is a symbol in /v1/symbols/{packagePath}.
type Symbol struct {
	ModulePath string `json:"modulePath"`
	Version    string `json:"version"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Synopsis   string `json:"synopsis"`
	Parent     string `json:"parent,omitempty"`
}

// SearchResults is the response for /v1/search?q={query}.
type SearchResult struct {
	PackagePath string `json:"packagePath"`
	ModulePath  string `json:"modulePath"`
	Version     string `json:"version"`
	Synopsis    string `json:"synopsis"`
}

// Vulnerability is a vulnerability in /v1/vulnerabilities/{modulePath}.
type Vulnerability struct {
	ID           string `json:"id"`
	Summary      string `json:"summary"`
	Details      string `json:"details"`
	FixedVersion string `json:"fixedVersion"`
}

// Error contains detailed information about an error.
type Error struct {
	Code       int         `json:"code"`
	Message    string      `json:"message"`
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
func BadRequest(format string, args ...any) *Error {
	return &Error{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf(format, args...),
	}
}

// InternalServerError returns an Error with StatusInternalServerError.
func InternalServerError(format string, args ...any) *Error {
	return &Error{
		Code:    http.StatusInternalServerError,
		Message: strings.ToLower(http.StatusText(http.StatusInternalServerError)),
		err:     fmt.Errorf(format, args...),
	}
}

// Candidate is a potential resolution for an ambiguous path.
type Candidate struct {
	ModulePath  string `json:"modulePath"`
	PackagePath string `json:"packagePath"`
}
