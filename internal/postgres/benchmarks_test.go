// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/database"
)

var testQueries = []string{
	"github",
	"cloud",
	"golang",
	"go",
	"mutex",
	"elasticsearch",
	"errors",
	"kubernetes",
	"github golang",
	"hashicorp",
	"test",
	"teest",
	"imports",
	"net",
	"s3blob",
	"k8s",
}

func BenchmarkSearch(b *testing.B) {
	ctx := context.Background()
	cfg, err := config.Init(ctx)
	if err != nil {
		b.Fatal(err)
	}
	ddb, err := database.Open("pgx", cfg.DBConnInfo())
	if err != nil {
		b.Fatal(err)
	}
	db := New(ddb)
	searchers := map[string]func(context.Context, string, int, int) ([]*internal.SearchResult, error){
		"db.Search": db.Search,
	}
	for name, search := range searchers {
		for _, query := range testQueries {
			b.Run(name+":"+query, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if _, err := search(ctx, query, 10, 0); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
