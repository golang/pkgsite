// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
)

func TestETL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	start := postgres.NowTruncated()
	var (
		notFound            = http.StatusNotFound
		internalServerError = http.StatusInternalServerError
	)

	tests := []struct {
		label      string
		index      []*internal.IndexVersion
		fetch      map[fetch.Request]*fetch.Response
		requests   []*http.Request
		wantNext   []*internal.VersionState
		wantErrors []*internal.VersionState
	}{
		{
			label: "index update without fetch",
			index: []*internal.IndexVersion{
				{Path: "foo.com/bar", Timestamp: start, Version: "v1.0.0"},
			},
			fetch: map[fetch.Request]*fetch.Response{
				{ModulePath: "foo.com/bar", Version: "v1.0.0"}: {StatusCode: 200},
			},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/indexupdate/", nil),
			},
			wantNext: []*internal.VersionState{
				{ModulePath: "foo.com/bar", IndexTimestamp: start, Version: "v1.0.0"},
			},
		}, {
			label: "partial fetch",
			index: []*internal.IndexVersion{
				{Path: "foo.com/foo", Timestamp: start, Version: "v1.0.0"},
				{Path: "foo.com/bar", Timestamp: start.Add(time.Second), Version: "v0.0.1"},
			},
			fetch: map[fetch.Request]*fetch.Response{
				{ModulePath: "foo.com/bar", Version: "v0.0.1"}: {StatusCode: 200},
			},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/indexupdate/", nil),
				httptest.NewRequest("POST", "/fetchversions/?limit=1", nil),
			},
			wantNext: []*internal.VersionState{
				{ModulePath: "foo.com/foo", IndexTimestamp: start, Version: "v1.0.0"},
			},
		}, {
			label: "fetch with errors",
			index: []*internal.IndexVersion{
				{Path: "foo.com/foo", Timestamp: start, Version: "v1.0.0"},
				{Path: "foo.com/bar", Timestamp: start.Add(time.Second), Version: "v0.0.1"},
			},
			fetch: map[fetch.Request]*fetch.Response{
				{ModulePath: "foo.com/bar", Version: "v0.0.1"}: {StatusCode: 500},
			},
			requests: []*http.Request{
				httptest.NewRequest("POST", "/indexupdate/", nil),
				httptest.NewRequest("POST", "/fetchversions/", nil),
			},
			wantErrors: []*internal.VersionState{{
				ModulePath:     "foo.com/bar",
				IndexTimestamp: start.Add(time.Second),
				Version:        "v0.0.1",
				TryCount:       1,
				Status:         &internalServerError,
			}, {
				ModulePath:     "foo.com/foo",
				IndexTimestamp: start,
				Version:        "v1.0.0",
				TryCount:       1,
				Status:         &notFound,
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			teardownIndex, indexClient := index.SetupTestIndex(t, test.index)
			defer teardownIndex(t)
			teardownFetch, fetchClient := fetch.SetupTestFetch(t, test.fetch)
			defer teardownFetch(t)
			defer postgres.ResetTestDB(testDB, t)

			// Use 10 workers to have parallelism consistent with the cron binary.
			s := NewServer(testDB, indexClient, fetchClient, nil, 10)

			for _, r := range test.requests {
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				if got, want := w.Code, http.StatusOK; got != want {
					t.Fatalf("Code = %d, want %d", got, want)
				}
			}

			// for the upcoming checks, query a large enough number of versions to
			// capture all state resulting from the test operations.
			const versionsToFetch = 100

			got, err := testDB.GetNextVersionsToFetch(ctx, versionsToFetch)
			if err != nil {
				t.Fatalf("testDB.GetNextVersionsToFetch(ctx, %d): %v", versionsToFetch, err)
			}
			ignore := cmpopts.IgnoreFields(internal.VersionState{}, "CreatedAt", "NextProcessedAfter", "LastProcessedAt", "Error")
			if diff := cmp.Diff(test.wantNext, got, ignore); diff != "" {
				t.Errorf("testDB.GetNextVersionsToFetch(ctx, %d) mismatch (-want +got):\n%s", versionsToFetch, diff)
			}

			got, err = testDB.GetRecentFailedVersions(ctx, versionsToFetch)
			if err != nil {
				t.Fatalf("testDB.GetRecentFailedVersions(ctx, %d): %v", versionsToFetch, err)
			}
			sort.Slice(got, func(i, j int) bool {
				return got[i].ModulePath < got[j].ModulePath
			})
			if diff := cmp.Diff(test.wantErrors, got, ignore); diff != "" {
				t.Errorf("testDB.GetRecentFailedVersions(ctx, 100) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
