// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package searcheval

import (
	"context"
	"errors"
	"math"
	"testing"
)

type mockSearcher struct {
	results map[string][]SearchResult
	err     error
}

func (m *mockSearcher) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results[query], nil
}

func TestRunEvaluation(t *testing.T) {
	queries := []GoldenQuery{
		{
			Query: "http client",
			ExpectedPackages: []ExpectedPackage{
				{PackagePath: "net/http"},
			},
		},
		{
			Query: "json parser",
			ExpectedPackages: []ExpectedPackage{
				{PackagePath: "encoding/json"},
			},
		},
	}

	searcher := &mockSearcher{
		results: map[string][]SearchResult{
			"http client": {
				{PackagePath: "net/http"},
			},
			"json parser": {
				{PackagePath: "github.com/foo/bar"},
				{PackagePath: "encoding/json"},
			},
		},
	}

	opts := SearchOptions{Limit: 5}

	t.Run("successful evaluation", func(t *testing.T) {
		report, err := RunEvaluation(context.Background(), searcher, queries, opts, opts, opts)
		if err != nil {
			t.Fatalf("RunEvaluation error: %v", err)
		}

		if len(report.QueryReports) != 2 {
			t.Fatalf("len(QueryReports) = %d, want 2", len(report.QueryReports))
		}

		// http client: net/http is rank 1 -> RR = 1.0
		if math.Abs(report.QueryReports[0].KeywordRR-1.0) > 1e-6 {
			t.Errorf("QueryReports[0].KeywordRR = %f, want 1.0", report.QueryReports[0].KeywordRR)
		}

		// json parser: encoding/json is rank 2 -> RR = 0.5
		if math.Abs(report.QueryReports[1].KeywordRR-0.5) > 1e-6 {
			t.Errorf("QueryReports[1].KeywordRR = %f, want 0.5", report.QueryReports[1].KeywordRR)
		}

		// Mean = (1.0 + 0.5) / 2 = 0.75
		if math.Abs(report.MeanKeyword-0.75) > 1e-6 {
			t.Errorf("report.MeanKeyword = %f, want 0.75", report.MeanKeyword)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel context immediately

		_, err := RunEvaluation(ctx, searcher, queries, opts, opts, opts)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("RunEvaluation on canceled context = %v, want context.Canceled", err)
		}
	})

	t.Run("search error propagation", func(t *testing.T) {
		errSearch := errors.New("database connection failed")
		failingSearcher := &mockSearcher{err: errSearch}

		report, err := RunEvaluation(context.Background(), failingSearcher, queries, opts, opts, opts)
		if err != nil {
			t.Fatalf("RunEvaluation error: %v", err)
		}

		if len(report.QueryReports) != 2 {
			t.Fatalf("len(QueryReports) = %d, want 2", len(report.QueryReports))
		}

		if report.QueryReports[0].Error == nil {
			t.Errorf("QueryReports[0].Error = nil, want error")
		}
	})
}
