// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"html/template"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		module  string
		version string
		err     bool
	}{
		{
			name:    "valid_url",
			url:     "https://discovery.com/test.module@v1.0.0",
			module:  "test.module",
			version: "v1.0.0",
		},
		{
			name:    "valid_url_with_tab",
			url:     "https://discovery.com/test.module@v1.0.0?tab=docs",
			module:  "test.module",
			version: "v1.0.0",
		},
		{
			name: "invalid_url",
			url:  "https://discovery.com/",
			err:  true,
		},
		{
			name: "invalid_url_missing_module",
			url:  "https://discovery.com@v1.0.0",
			err:  true,
		},
		{
			name: "invalid_url_missing_version",
			url:  "https://discovery.com/module",
			err:  true,
		},
		{
			name: "invalid_version",
			url:  "https://discovery.com/module@v1.0.0invalid",
			err:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			module, version, err := parseModulePathAndVersion(u.Path)

			if (err != nil) != tc.err {
				t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want error %t)", u, err, tc.err)
			}

			if !tc.err && (tc.module != module || tc.version != version) {
				t.Fatalf("parseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, want err %t",
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
		name        string
		version     internal.Version
		wantModPage ModulePage
	}{

		name: "want_expected_module_page",
		version: internal.Version{
			Module: &internal.Module{
				Path: "test.com/module",
				Series: &internal.Series{
					Path: "series",
				},
			},
			Version:    "v1.0.0",
			Synopsis:   "test synopsis",
			CommitTime: time.Now().Add(time.Hour * -8),
			License:    "MIT",
			ReadMe:     []byte("This is the readme text."),
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "pkg_name",
					Path:     "test.com/module/pkg_name",
					Synopsis: "Test package synopsis",
				},
			},
			VersionType: internal.VersionTypeRelease,
		},
		wantModPage: ModulePage{
			ModulePath: "test.com/module",
			ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
			PackageHeader: &PackageHeader{
				Name:       "pkg_name",
				Version:    "v1.0.0",
				Path:       "test.com/module/pkg_name",
				Synopsis:   "Test package synopsis",
				License:    "MIT",
				CommitTime: "today",
			},
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(&tc.version); err != nil {
		t.Fatalf("db.InsertVersion(%v) returned error: %v", tc.version, err)
	}

	got, err := fetchModulePage(db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchModulePage(db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantModPage)
	}

	if diff := cmp.Diff(tc.wantModPage, *got); diff != "" {
		t.Errorf("fetchModulePage(%q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name, readme string
		want         template.HTML
	}{
		{
			name: "valid_markdown_readme",
			readme: "This package collects pithy sayings.\n\n" +
				"It's part of a demonstration of\n" +
				"[package versioning in Go](https://research.swtch.com/vgo1).",
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name:   "empty_readme",
			readme: "",
			want:   template.HTML(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML([]byte(tc.readme))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%q) mismatch (-want +got):\n%s", tc.readme, diff)
			}
		})
	}
}
