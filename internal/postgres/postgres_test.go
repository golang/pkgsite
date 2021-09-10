// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/pkgsite/internal/derrors"
)

const testTimeout = 5 * time.Second

var acquire func(*testing.T) (*DB, func())

func TestMain(m *testing.M) {
	startPoller = false
	RunDBTestsInParallel("discovery_postgres_test", 4, m, &acquire)
}

func TestGetOldestUnprocessedIndexTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	type modTimes struct {
		indexTimestamp  string // time in Kitchen format
		lastProcessedAt string // ditto, empty means NULL
	}

	for _, test := range []struct {
		name string
		mods []modTimes
		want string // empty => error
	}{
		{
			"no modules",
			nil,
			"",
		},
		{
			"no unprocessed modules",
			[]modTimes{
				{"7:00AM", "7:02AM"}, // index says 7am, processed at 7:02
			},
			"",
		},
		{
			"no processed modules",
			[]modTimes{
				{"7:00AM", ""}, // index says 7am, never processed
			},
			"",
		},
		{
			"several modules",
			[]modTimes{
				{"5:00AM", ""}, // old, never processed
				{"6:00AM", "6:35AM"},
				{"7:00AM", "7:02AM"}, // youngest processed module
				{"8:00AM", ""},       // oldest unprocessed after youngest processed
				{"9:00AM", ""},
			},
			"8:00AM",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()
			for i, m := range test.mods {
				path := fmt.Sprintf("m%d", i)
				it, err := time.Parse(time.Kitchen, m.indexTimestamp)
				if err != nil {
					t.Fatal(err)
				}
				var lpt *time.Time
				if m.lastProcessedAt != "" {
					p, err := time.Parse(time.Kitchen, m.lastProcessedAt)
					if err != nil {
						t.Fatal(err)
					}
					lpt = &p
				}
				if _, err := testDB.db.Exec(ctx, `
					INSERT INTO module_version_states (module_path, version, index_timestamp, last_processed_at, sort_version, incompatible)
					VALUES ($1, 'v1.0.0', $2, $3, 'x', false)
				`, path, it, lpt); err != nil {
					t.Fatal(err)
				}
			}
			got, err := testDB.StalenessTimestamp(ctx)
			if err != nil && errors.Is(err, derrors.NotFound) {
				if test.want != "" {
					t.Fatalf("got unexpected error %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else {
				want, err := time.Parse(time.Kitchen, test.want)
				if err != nil {
					t.Fatal(err)
				}
				if !got.Equal(want) {
					t.Errorf("got %s, want %s", got, want)
				}
			}
		})
	}
}

func TestGetUserInfo(t *testing.T) {
	// We can't know what we'll get from this query, so just perform some basic
	// sanity checks.
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	got, err := testDB.GetUserInfo(ctx, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got.NumTotal < 1 {
		t.Errorf("total = %d, wanted >= 1", got.NumTotal)
	}
}
