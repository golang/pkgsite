// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

func TestParseNameAndVersion(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		module  string
		version string
		err     bool
	}{
		{
			name:    "valid_url",
			url:     "https://discovery.com/module?v=v1.0.0",
			module:  "module",
			version: "v1.0.0",
		},
		{
			name:    "valid_url_with_tab",
			url:     "https://discovery.com/module?v=v1.0.0&tab=docs",
			module:  "module",
			version: "v1.0.0",
		},
		{
			name: "invalid_url",
			url:  "https://discovery.com/",
			err:  true,
		},
		{
			name: "invalid_url_missing_module",
			url:  "https://discovery.com?v=v1.0.0",
			err:  true,
		},
		{
			name: "invalid_url_missing_version",
			url:  "https://discovery.com/module",
			err:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			module, version, err := parseNameAndVersion(u)

			if (err != nil) != tc.err {
				t.Fatalf("parseNameAndVersion(%v) error = (%v); want error %t)", u, err, tc.err)
			}

			if !tc.err && (tc.module != module || tc.version != version) {
				t.Fatalf("parseNameAndVersion(%v): %q, %q, %v; want = %q, %q, want err %t",
					u, module, version, err, tc.module, tc.version, tc.err)
			}
		})
	}
}

func TestElapsedTime(t *testing.T) {
	now := time.Now()
	testCases := []struct {
		name        string
		date        time.Time
		elapsedTime string
	}{
		{
			name:        "one_hour_ago",
			date:        now.Add(time.Hour * -1),
			elapsedTime: "1 hour ago",
		},
		{
			name:        "hours_ago",
			date:        now.Add(time.Hour * -2),
			elapsedTime: "2 hours ago",
		},
		{
			name:        "today",
			date:        now.Add(time.Hour * -8),
			elapsedTime: "today",
		},
		{
			name:        "one_day_ago",
			date:        now.Add(time.Hour * 24 * -1),
			elapsedTime: "1 day ago",
		},
		{
			name:        "days_ago",
			date:        now.Add(time.Hour * 24 * -5),
			elapsedTime: "5 days ago",
		},
		{
			name:        "more_than_6_days_ago",
			date:        now.Add(time.Hour * 24 * -14),
			elapsedTime: now.Add(time.Hour * 24 * -14).Format("Jan _2, 2006"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			elapsedTime := elapsedTime(tc.date)

			if elapsedTime != tc.elapsedTime {
				t.Errorf("elapsedTime(%q) = %s, want %s", tc.date, elapsedTime, tc.elapsedTime)
			}
		})
	}
}

func TestFetchModulePage(t *testing.T) {
	tc := struct {
		name            string
		version         internal.Version
		expectedModPage ModulePage
	}{

		name: "want_expected_module_page",
		version: internal.Version{
			Module: &internal.Module{
				Name: "test/module",
				Series: &internal.Series{
					Name: "series",
				},
			},
			Version:    "v1.0.0",
			Synopsis:   "test synopsis",
			CommitTime: time.Now().Add(time.Hour * -8),
			License:    "MIT",
			ReadMe:     "This is the readme text.",
		},
		expectedModPage: ModulePage{
			Name:       "test/module",
			Version:    "v1.0.0",
			License:    "MIT",
			CommitTime: "today",
			ReadMe:     "This is the readme text.",
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(&tc.version); err != nil {
		t.Fatalf("db.InsertVersion(&%q) returned error: %v", tc.version, err)
	}

	mp, err := fetchModulePage(db, tc.version.Module.Name, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchModulePage(db, %q, %q) = %v err = %v, want %v",
			tc.version.Module.Name, tc.version.Version, mp, err, tc.expectedModPage)
	}

	if !reflect.DeepEqual(*mp, tc.expectedModPage) {
		t.Errorf("reflect.DeepEqual(%q, %q) was false, want true", *mp, tc.expectedModPage)
	}
}
