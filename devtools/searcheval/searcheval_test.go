// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package searcheval

import "testing"

func TestReciprocalRank(t *testing.T) {
	results := []SearchResult{
		{PackagePath: "github.com/foo/bar"},
		{PackagePath: "github.com/go-chi/chi/v5"},
		{PackagePath: "github.com/gorilla/mux"},
	}

	tests := []struct {
		name       string
		targetPath string
		wantRank   int
		wantRR     float64
	}{
		{
			name:       "rank 1 match",
			targetPath: "github.com/foo/bar",
			wantRank:   1,
			wantRR:     1.0,
		},
		{
			name:       "rank 2 match",
			targetPath: "github.com/go-chi/chi/v5",
			wantRank:   2,
			wantRR:     0.5,
		},
		{
			name:       "rank 3 match",
			targetPath: "github.com/gorilla/mux",
			wantRank:   3,
			wantRR:     1.0 / 3.0,
		},
		{
			name:       "not found",
			targetPath: "github.com/missing/pkg",
			wantRank:   0,
			wantRR:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRank, gotRR := ReciprocalRank(results, tt.targetPath)
			if gotRank != tt.wantRank || gotRR != tt.wantRR {
				t.Errorf("ReciprocalRank(%q) = (%d, %f), want (%d, %f)", tt.targetPath, gotRank, gotRR, tt.wantRank, tt.wantRR)
			}
		})
	}
}
