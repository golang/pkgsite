// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package searcheval

import (
	"context"
	"slices"
)

// SearchResult represents a single item returned by a searcher.
type SearchResult struct {
	PackagePath     string
	Score           float64
	ImportedByCount int
}

// SearchMode defines the execution mode for search queries.
type SearchMode string

const (
	ModeKeyword SearchMode = "keyword"
	ModeVector  SearchMode = "vector"
	ModeHybrid  SearchMode = "hybrid"
)

// SearchOptions wraps scoring weights and query inputs.
type SearchOptions struct {
	Mode  SearchMode // ModeKeyword, ModeVector, ModeHybrid
	Limit int
	// TextWeights represents PostgreSQL ts_rank section weights [D, C, B, A]
	// mapping to [Description, Synopsis/Header, Symbol, Path/Title].
	TextWeights      [4]float64
	PopularityWeight float64
	VectorWeight     float64
}

// Searcher abstracts search engines (postgres, HTTP client, mocks).
type Searcher interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

// ExpectedPackage represents a target package expected in search results.
type ExpectedPackage struct {
	PackagePath string `json:"package_path"`
}

// GoldenQuery represents a test case.
type GoldenQuery struct {
	Query            string            `json:"query"`
	ExpectedPackages []ExpectedPackage `json:"expected_packages"`
}

// ReciprocalRank computes the 1-based rank and Reciprocal Rank (RR) metric for the target package.
// See go/pkgsite-search-mrr for rationale.
func ReciprocalRank(results []SearchResult, targetPath string) (int, float64) {
	idx := slices.IndexFunc(results, func(r SearchResult) bool {
		return r.PackagePath == targetPath
	})
	if idx != -1 {
		rank := idx + 1
		return rank, 1.0 / float64(rank)
	}
	return 0, 0.0
}
