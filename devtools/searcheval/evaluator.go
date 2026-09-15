// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package searcheval

import (
	"context"
	"encoding/json"
	"os"
)

// QueryReport holds search metric evaluation results and any error encountered for a single query.
type QueryReport struct {
	Query       string
	KeywordRank int
	KeywordRR   float64
	VectorRank  int
	VectorRR    float64
	HybridRank  int
	HybridRR    float64
	Error       error
}

// EvalReport summarizes aggregate search evaluation results and metric averages across a golden query dataset.
type EvalReport struct {
	QueryReports []QueryReport
	MeanKeyword  float64
	MeanVector   float64
	MeanHybrid   float64
}

// LoadGoldenQueries parses queries from a JSON configuration file.
func LoadGoldenQueries(filepath string) ([]GoldenQuery, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	var queries []GoldenQuery
	if err := json.Unmarshal(data, &queries); err != nil {
		return nil, err
	}
	return queries, nil
}

// RunEvaluation executes the golden query set against keyword, vector, and hybrid search options.
func RunEvaluation(ctx context.Context, s Searcher, queries []GoldenQuery, textOpts, vecOpts, hybOpts SearchOptions) (*EvalReport, error) {
	report := &EvalReport{}
	var sumKeyword, sumVector, sumHybrid float64

	for _, g := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		targetPkg := ""
		if len(g.ExpectedPackages) > 0 {
			targetPkg = g.ExpectedPackages[0].PackagePath
		}

		qr := QueryReport{Query: g.Query}

		textOpts.Mode = ModeKeyword
		kwRes, err := s.Search(ctx, g.Query, textOpts)
		if err != nil {
			qr.Error = err
		} else {
			qr.KeywordRank, qr.KeywordRR = ReciprocalRank(kwRes, targetPkg)
			sumKeyword += qr.KeywordRR
		}

		vecOpts.Mode = ModeVector
		vecRes, err := s.Search(ctx, g.Query, vecOpts)
		if err != nil && qr.Error == nil {
			qr.Error = err
		} else if err == nil {
			qr.VectorRank, qr.VectorRR = ReciprocalRank(vecRes, targetPkg)
			sumVector += qr.VectorRR
		}

		hybOpts.Mode = ModeHybrid
		hybRes, err := s.Search(ctx, g.Query, hybOpts)
		if err != nil && qr.Error == nil {
			qr.Error = err
		} else if err == nil {
			qr.HybridRank, qr.HybridRR = ReciprocalRank(hybRes, targetPkg)
			sumHybrid += qr.HybridRR
		}

		report.QueryReports = append(report.QueryReports, qr)
	}

	count := float64(len(queries))
	if count > 0 {
		report.MeanKeyword = sumKeyword / count
		report.MeanVector = sumVector / count
		report.MeanHybrid = sumHybrid / count
	}

	return report, nil
}
