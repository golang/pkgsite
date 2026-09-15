// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/pkgsite/devtools/searcheval"
	"golang.org/x/pkgsite/internal/embeddings"
	"golang.org/x/pkgsite/internal/postgres"
)

// dbSearcher implements searcheval.Searcher.
type dbSearcher struct {
	db     *postgres.DB
	client *embeddings.Client
}

func (s *dbSearcher) Search(ctx context.Context, query string, opts searcheval.SearchOptions) ([]searcheval.SearchResult, error) {
	pgOpts := postgres.SearchOptions{
		MaxResults: opts.Limit,
		ScoringParams: postgres.SearchScoringParams{
			TextWeights:      opts.TextWeights,
			PopularityWeight: opts.PopularityWeight,
			VectorWeight:     opts.VectorWeight,
		},
	}

	// Route based on Mode
	switch opts.Mode {
	case searcheval.ModeKeyword:
		dbresults, err := s.db.Search(ctx, query, pgOpts)
		if err != nil {
			return nil, err
		}
		return mapDBSearchResults(dbresults), nil

	case searcheval.ModeVector:
		// Direct nearest-neighbors query from database (offline/manual pgvector implementation)
		vectors, err := s.client.GenerateEmbeddings(ctx, []string{query}, embeddings.TaskTypeQuery)
		if err != nil {
			return nil, err
		}
		vecStr := formatVector(vectors[0])

		sqlQuery := `
			WITH nearest_neighbors AS (
				SELECT package_path, imported_by_count, redistributable, has_go_mod,
				       (1.0 - (embedding <=> $1::halfvec)) AS similarity
				FROM search_documents
				WHERE embedding IS NOT NULL
				ORDER BY embedding <=> $1::halfvec
				LIMIT 100
			)
			SELECT package_path,
			       (similarity * ln(exp(1)+imported_by_count) *
			        CASE WHEN redistributable THEN 1 ELSE 0.5 END *
			        CASE WHEN COALESCE(has_go_mod, true) THEN 1 ELSE 0.8 END) AS score,
				   imported_by_count
			FROM nearest_neighbors
			ORDER BY score DESC, imported_by_count DESC, package_path;`

		var results []searcheval.SearchResult
		collect := func(rows *sql.Rows) error {
			var r searcheval.SearchResult
			if err := rows.Scan(&r.PackagePath, &r.Score, &r.ImportedByCount); err != nil {
				return err
			}
			results = append(results, r)
			return nil
		}
		if err := s.db.Underlying().RunQuery(ctx, sqlQuery, collect, vecStr); err != nil {
			return nil, err
		}
		return results, nil

	case searcheval.ModeHybrid:
		vectors, err := s.client.GenerateEmbeddings(ctx, []string{query}, embeddings.TaskTypeQuery)
		if err != nil {
			return nil, err
		}
		pgOpts.Vector = vectors[0]

		dbresults, err := s.db.Search(ctx, query, pgOpts)
		if err != nil {
			return nil, err
		}
		return mapDBSearchResults(dbresults), nil

	default:
		return nil, fmt.Errorf("unknown search mode: %q", opts.Mode)
	}
}

func mapDBSearchResults(dbresults []*postgres.SearchResult) []searcheval.SearchResult {
	var results []searcheval.SearchResult
	for _, r := range dbresults {
		results = append(results, searcheval.SearchResult{
			PackagePath:     r.PackagePath,
			Score:           r.Score,
			ImportedByCount: int(r.NumImportedBy),
		})
	}
	return results
}

func formatVector(vec []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", v)
	}
	sb.WriteByte(']')
	return sb.String()
}
