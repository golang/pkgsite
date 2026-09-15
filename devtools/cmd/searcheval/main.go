// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"contrib.go.opencensus.io/integrations/ocsql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/serverconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/embeddings"
	"golang.org/x/pkgsite/internal/postgres"
)

func openDB(cfg *config.Config) (*postgres.DB, error) {
	ocDriver, err := database.RegisterOCWrapper("pgx", ocsql.WithAllTraceOptions())
	if err != nil {
		return nil, fmt.Errorf("unable to register the ocsql driver: %v", err)
	}
	ddb, err := database.Open(ocDriver, cfg.DBConnInfo(), cfg.InstanceID)
	if err != nil {
		ci := cfg.DBSecondaryConnInfo()
		if ci == "" {
			return nil, err
		}
		ddb, err = database.Open(ocDriver, ci, cfg.InstanceID)
		if err != nil {
			return nil, err
		}
	}
	ddb.SetPoolSettings(cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime, cfg.DBConnMaxIdleTime)
	return postgres.NewBypassingLicenseCheck(ddb), nil
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `searcheval - Search Quality Evaluation & Tuning Tool

Usage:
  searcheval <command> [flags] [args]

Commands:
  eval       Run MRR@5 quality evaluation against ground-truth golden queries dataset.
  log-eval   Run Keyword vs Hybrid comparative evaluation against raw query logs list.
  search <q> Run interactive side-by-side search debugging for a query string.
  embed      Generate Vertex AI embeddings for packages in search_documents.
  seed [mods] Seed 100 popular Go modules (or custom modules passed as args) into a local PostgreSQL instance.
  help       Show this help guide.

Flags (for eval, log-eval, search):
  -queries <path>
    	Path to golden queries JSON file (default: testdata/golden_queries.json)
  -log-queries <path>
    	Path to raw query log list file (default: private/devtools/cmd/search/20210721-10m.queries)
  -text-weights <D,C,B,A>
    	Comma-separated ts_rank section weights for keyword search (e.g. "0.1,0.2,1.0,1.0")
  -popularity-weight <float>
    	Import count popularity exponent parameter (default: uses DB config)
  -vector-weight <float>
    	Reciprocal Rank Fusion (RRF) vector weight (default: uses DB config)
  -limit <int>
    	Number of search results to display per engine (default: 10)

Examples:
  # Run MRR evaluation on golden queries dataset
  go run ./devtools/cmd/searcheval eval

  # Run evaluation with custom vector RRF weight
  go run ./devtools/cmd/searcheval eval -vector-weight=1.5

  # Run comparative evaluation on production query logs
  go run ./devtools/cmd/searcheval log-eval

  # Interactive search comparison for a query string
  go run ./devtools/cmd/searcheval search "structured logger"

  # Seed custom modules into local database
  go run ./devtools/cmd/searcheval seed github.com/go-chi/chi/v5@v5.0.12 github.com/gin-gonic/gin@v1.9.1
`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	cmd := os.Args[1]
	if cmd == "help" || cmd == "-help" || cmd == "--help" || cmd == "-h" {
		printHelp()
		os.Exit(0)
	}

	ctx := context.Background()
	cfg, err := serverconfig.Init(ctx)
	if err != nil {
		log.Fatalf("serverconfig.Init: %v", err)
	}

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	cfg.EnableVectorSearch = true
	if cfg.LocationID == "" {
		cfg.LocationID = "us-central1"
	}
	client, err := embeddings.NewClient(ctx, cfg)
	if err != nil {
		log.Fatalf("embeddings.NewClient (Vertex AI): %v", err)
	}

	searcher := &dbSearcher{db: db, client: client}

	switch cmd {
	case "eval":
		evalCmd := flag.NewFlagSet("eval", flag.ExitOnError)
		queriesFile := evalCmd.String("queries", "devtools/cmd/searcheval/testdata/golden_queries.json", "path to golden queries JSON file")
		textWeights := evalCmd.String("text-weights", "", "comma-separated ts_rank weights (D,C,B,A)")
		popWeight := evalCmd.Float64("popularity-weight", -1.0, "popularity exponent parameter")
		vectorWeight := evalCmd.Float64("vector-weight", -1.0, "RRF vector search weight")
		evalCmd.Parse(os.Args[2:])

		scoringParams := parseScoringParams(*textWeights, *popWeight, *vectorWeight)
		runEvalCmd(ctx, searcher, *queriesFile, scoringParams)

	case "log-eval", "prod-eval":
		logEvalCmd := flag.NewFlagSet("log-eval", flag.ExitOnError)
		logQueries := logEvalCmd.String("log-queries", "private/devtools/cmd/search/20210721-10m.queries", "path to raw query log list file")
		textWeights := logEvalCmd.String("text-weights", "", "comma-separated ts_rank weights (D,C,B,A)")
		popWeight := logEvalCmd.Float64("popularity-weight", -1.0, "popularity exponent parameter")
		vectorWeight := logEvalCmd.Float64("vector-weight", -1.0, "RRF vector search weight")
		logEvalCmd.Parse(os.Args[2:])

		scoringParams := parseScoringParams(*textWeights, *popWeight, *vectorWeight)
		runLogEvalCmd(ctx, searcher, *logQueries, scoringParams)

	case "search":
		searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
		limit := searchCmd.Int("limit", 10, "number of search results to display")
		textWeights := searchCmd.String("text-weights", "", "comma-separated ts_rank weights (D,C,B,A)")
		popWeight := searchCmd.Float64("popularity-weight", -1.0, "popularity exponent parameter")
		vectorWeight := searchCmd.Float64("vector-weight", -1.0, "RRF vector search weight")
		searchCmd.Parse(os.Args[2:])

		if searchCmd.NArg() < 1 {
			log.Fatalf("search command requires a query string argument")
		}
		query := strings.Join(searchCmd.Args(), " ")
		scoringParams := parseScoringParams(*textWeights, *popWeight, *vectorWeight)
		runSearchCmd(ctx, searcher, query, *limit, scoringParams)

	case "seed":
		seedCmd := flag.NewFlagSet("seed", flag.ExitOnError)
		seedCmd.Parse(os.Args[2:])
		runSeed(ctx, db, cfg, seedCmd.Args())

	case "embed":
		embedCmd := flag.NewFlagSet("embed", flag.ExitOnError)
		embedCmd.Parse(os.Args[2:])
		runEmbed(ctx, db, client)

	default:
		printHelp()
		os.Exit(1)
	}
}

func parseScoringParams(rawTextWeights string, popWeight, vectorWeight float64) postgres.SearchScoringParams {
	params := postgres.SearchScoringParams{
		TextWeights:      [4]float64{0.1, 0.2, 1.0, 1.0},
		PopularityWeight: 1.0,
		VectorWeight:     1.0,
	}

	if rawTextWeights != "" {
		parts := strings.Split(rawTextWeights, ",")
		if len(parts) != 4 {
			log.Fatalf("invalid text-weights format, must be 4 comma-separated values")
		}
		for i, p := range parts {
			val, err := strconv.ParseFloat(p, 64)
			if err != nil {
				log.Fatalf("invalid text-weight value: %v", err)
			}
			params.TextWeights[i] = val
		}
	}
	if popWeight >= 0 {
		params.PopularityWeight = popWeight
	}
	if vectorWeight >= 0 {
		params.VectorWeight = vectorWeight
	}
	return params
}
