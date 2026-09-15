// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"slices"
	"strings"

	"golang.org/x/pkgsite/devtools/searcheval"
	"golang.org/x/pkgsite/internal/embeddings"
	"golang.org/x/pkgsite/internal/postgres"
)

func runSearchCmd(ctx context.Context, searcher *dbSearcher, query string, displayLimit int, scoringParams postgres.SearchScoringParams) {
	log.Printf("Query: %q", query)

	opts := searcheval.SearchOptions{
		Limit:            100,
		TextWeights:      scoringParams.TextWeights,
		PopularityWeight: scoringParams.PopularityWeight,
		VectorWeight:     scoringParams.VectorWeight,
	}

	opts.Mode = searcheval.ModeKeyword
	keywordResults, err := searcher.Search(ctx, query, opts)
	if err != nil {
		log.Printf("Keyword Search error: %v", err)
	}

	opts.Mode = searcheval.ModeVector
	vectorResults, err := searcher.Search(ctx, query, opts)
	if err != nil {
		log.Printf("Vector Search error: %v", err)
	}

	opts.Mode = searcheval.ModeHybrid
	hybridResults, err := searcher.Search(ctx, query, opts)
	if err != nil {
		log.Printf("Hybrid Search error: %v", err)
	}

	fmt.Printf("\n%-40s | %-40s | %-40s\n", "KEYWORD RESULTS", "VECTOR RESULTS", "HYBRID (RRF) RESULTS")
	fmt.Println(strings.Repeat("-", 130))

	maxLen := displayLimit
	for i := 0; i < maxLen; i++ {
		kStr := "-"
		if i < len(keywordResults) {
			kStr = fmt.Sprintf("%s (%.3f)", shortPath(keywordResults[i].PackagePath), keywordResults[i].Score)
		}

		vStr := "-"
		if i < len(vectorResults) {
			vStr = fmt.Sprintf("%s (%.3f)", shortPath(vectorResults[i].PackagePath), vectorResults[i].Score)
		}

		hStr := "-"
		if i < len(hybridResults) {
			hStr = fmt.Sprintf("%s (%.3f)", shortPath(hybridResults[i].PackagePath), hybridResults[i].Score)
		}

		fmt.Printf("%-40s | %-40s | %-40s\n", kStr, vStr, hStr)
	}
}

func shortPath(p string) string {
	if len(p) > 35 {
		return "..." + p[len(p)-32:]
	}
	return p
}

func resolveQueriesFile(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	altPaths := []string{
		"devtools/cmd/searcheval/testdata/golden_queries.json",
		"private/devtools/cmd/searcheval/testdata/golden_queries.json",
		"cmd/searcheval/testdata/golden_queries.json",
		"testdata/golden_queries.json",
	}
	for _, alt := range altPaths {
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return path
}

func formatRankRR(rank int, rr float64) string {
	if rank == 0 {
		return fmt.Sprintf(">5 (%.3f)", rr)
	}
	return fmt.Sprintf("#%d (%.3f)", rank, rr)
}

func runEvalCmd(ctx context.Context, searcher *dbSearcher, queriesPath string, scoringParams postgres.SearchScoringParams) {
	qFile := resolveQueriesFile(queriesPath)
	queries, err := searcheval.LoadGoldenQueries(qFile)
	if err != nil {
		log.Fatalf("LoadGoldenQueries failed: %v", err)
	}

	fmt.Printf("Loaded %d golden queries from %s\n", len(queries), qFile)
	fmt.Println("Evaluating search quality (MRR@5 metric)...")
	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("%-25s | %-35s | %-11s | %-11s | %-11s\n", "Query", "Target Package", "Keyword", "Vector", "Hybrid")
	fmt.Println(strings.Repeat("-", 90))

	textOpts := searcheval.SearchOptions{
		Limit:            5,
		TextWeights:      scoringParams.TextWeights,
		PopularityWeight: scoringParams.PopularityWeight,
		VectorWeight:     scoringParams.VectorWeight,
	}
	vecOpts := searcheval.SearchOptions{
		Limit:            5,
		TextWeights:      scoringParams.TextWeights,
		PopularityWeight: scoringParams.PopularityWeight,
		VectorWeight:     scoringParams.VectorWeight,
	}
	hybOpts := searcheval.SearchOptions{
		Limit:            5,
		TextWeights:      scoringParams.TextWeights,
		PopularityWeight: scoringParams.PopularityWeight,
		VectorWeight:     scoringParams.VectorWeight,
	}

	report, err := searcheval.RunEvaluation(ctx, searcher, queries, textOpts, vecOpts, hybOpts)
	if err != nil {
		log.Fatalf("RunEvaluation failed: %v", err)
	}

	for i, qr := range report.QueryReports {
		targetPkg := ""
		if len(queries[i].ExpectedPackages) > 0 {
			targetPkg = queries[i].ExpectedPackages[0].PackagePath
		}
		fmt.Printf("%-25s | %-35s | %-11s | %-11s | %-11s\n",
			qr.Query, shortPath(targetPkg),
			formatRankRR(qr.KeywordRank, qr.KeywordRR),
			formatRankRR(qr.VectorRank, qr.VectorRR),
			formatRankRR(qr.HybridRank, qr.HybridRR))
	}

	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("%-25s | %-35s | %-11.3f | %-11.3f | %-11.3f\n",
		"MEAN RECIPROCAL RANK (MRR)", "", report.MeanKeyword, report.MeanVector, report.MeanHybrid)
}

func runLogEvalCmd(ctx context.Context, searcher *dbSearcher, logQueriesPath string, scoringParams postgres.SearchScoringParams) {
	file, err := os.Open(logQueriesPath)
	if err != nil {
		log.Fatalf("failed to open query log file %q: %v", logQueriesPath, err)
	}
	defer file.Close()

	var queries []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		q := strings.TrimSpace(scanner.Text())
		if q != "" {
			queries = append(queries, q)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scanner error: %v", err)
	}

	log.Printf("Loaded %d query log strings from %s", len(queries), logQueriesPath)
	log.Println("Running Keyword vs Hybrid comparison on production logs...")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-35s | %-12s | %-12s | %-10s\n",
		"Query", "Key Results", "Hyb Results", "Overlap@5")
	fmt.Println(strings.Repeat("-", 80))

	var (
		totalQueries   int
		keyZeroResults int
		hybZeroResults int
		recoveredZero  int // keyword got 0, hybrid got > 0
		totalOverlap   float64
	)

	opts := searcheval.SearchOptions{
		Limit:            5,
		TextWeights:      scoringParams.TextWeights,
		PopularityWeight: scoringParams.PopularityWeight,
		VectorWeight:     scoringParams.VectorWeight,
	}

	for _, query := range queries {
		unescapedQuery, err := url.QueryUnescape(query)
		if err != nil {
			unescapedQuery = query
		}
		unescapedQuery = strings.ReplaceAll(unescapedQuery, "+", " ")

		totalQueries++

		// 1. Keyword Search
		opts.Mode = searcheval.ModeKeyword
		keyResults, err := searcher.Search(ctx, unescapedQuery, opts)
		if err != nil {
			log.Printf("Keyword Search failed for %q: %v", unescapedQuery, err)
			continue
		}

		// 2. Hybrid Search
		opts.Mode = searcheval.ModeHybrid
		hybResults, err := searcher.Search(ctx, unescapedQuery, opts)
		if err != nil {
			log.Printf("Hybrid Search failed for %q: %v", unescapedQuery, err)
			continue
		}

		// Calculate overlap
		keyPaths := make(map[string]bool)
		for _, r := range keyResults {
			keyPaths[r.PackagePath] = true
		}
		shared := 0
		for _, r := range hybResults {
			if keyPaths[r.PackagePath] {
				shared++
			}
		}
		overlap := 0.0
		if len(keyResults) > 0 || len(hybResults) > 0 {
			overlap = float64(shared) / float64(len(keyResults)+len(hybResults)-shared)
		}

		if len(keyResults) == 0 {
			keyZeroResults++
		}
		if len(hybResults) == 0 {
			hybZeroResults++
		}
		if len(keyResults) == 0 && len(hybResults) > 0 {
			recoveredZero++
		}
		totalOverlap += overlap

		fmt.Printf("%-35s | %-12d | %-12d | %-10.3f\n",
			shortPath(unescapedQuery), len(keyResults), len(hybResults), overlap)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("SUMMARY STATISTICS (%d queries total):\n", totalQueries)
	if totalQueries > 0 {
		fmt.Printf("Keyword Zero-Result Queries: %d (%.1f%%)\n", keyZeroResults, 100.0*float64(keyZeroResults)/float64(totalQueries))
		fmt.Printf("Hybrid Zero-Result Queries:  %d (%.1f%%)\n", hybZeroResults, 100.0*float64(hybZeroResults)/float64(totalQueries))
		fmt.Printf("Zero-Result Recoveries:      %d (%.1f%% of all queries, successfully surfaced via vector)\n",
			recoveredZero, 100.0*float64(recoveredZero)/float64(totalQueries))
		fmt.Printf("Average Result Overlap:      %.3f\n", totalOverlap/float64(totalQueries))
	} else {
		fmt.Println("No queries processed.")
	}
}

func runEmbed(ctx context.Context, db *postgres.DB, client *embeddings.Client) {
	log.Println("Scanning database for packages that need embeddings...")
	targets, err := db.GetPackagesToEmbed(ctx, 2000)
	if err != nil {
		log.Fatalf("GetPackagesToEmbed failed: %v", err)
	}

	if len(targets) == 0 {
		log.Println("No packages need embeddings (either imported_by_count < 1 or all already embedded).")
		return
	}

	processed := 0
	for batch := range slices.Chunk(targets, 20) {
		var prompts []string
		for _, t := range batch {
			prompts = append(prompts, fmt.Sprintf("Package: %s\nSynopsis: %s\nSymbols: %s\n", t.PackagePath, t.Synopsis, t.TopSymbols))
		}
		vectors, err := client.GenerateEmbeddings(ctx, prompts, embeddings.TaskTypeDocument)
		if err != nil {
			log.Fatalf("GenerateEmbeddings failed: %v", err)
		}
		for j, t := range batch {
			if err := db.UpdateSearchDocumentEmbedding(ctx, t.PackagePath, vectors[j]); err != nil {
				log.Fatalf("UpdateSearchDocumentEmbedding failed for %s: %v", t.PackagePath, err)
			}
		}
		processed += len(batch)
	}

	log.Printf("Successfully generated and saved embeddings for %d packages.", processed)
}
