// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

var (
	samplePackage = &Package{
		Name:       postgres.SamplePackage.Name,
		ModulePath: postgres.SampleModulePath,
		Version:    postgres.SampleVersionString,
		Path:       postgres.SamplePackage.Path,
		Synopsis:   postgres.SamplePackage.Synopsis,
		Licenses:   transformLicenseMetadata(postgres.SampleLicenseMetadata),
	}
)

func TestElapsedTime(t *testing.T) {
	now := postgres.NowTruncated()
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

// firstVersionedPackage is a helper function that returns an
// *internal.VersionedPackage corresponding to the first package in the
// version.
func firstVersionedPackage(v *internal.Version) *internal.VersionedPackage {
	return &internal.VersionedPackage{
		Package:     *v.Packages[0],
		VersionInfo: v.VersionInfo,
	}
}

func TestFetchOverviewDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *OverviewDetails
	}{
		name:    "want expected overview details",
		version: postgres.SampleVersion(),
		wantDetails: &OverviewDetails{
			ModulePath: postgres.SampleModulePath,
			ReadMe:     template.HTML("<p>readme</p>\n"),
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, postgres.SampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchOverviewDetails(ctx, testDB, firstVersionedPackage(tc.version))
	if err != nil {
		t.Fatalf("fetchOverviewDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetails)
	}

	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchOverviewDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name, readmeFilePath, readmeContents string
		want                                 template.HTML
	}{
		{
			name:           "valid markdown readme",
			readmeFilePath: "README.md",
			readmeContents: "This package collects pithy sayings.\n\n" +
				"It's part of a demonstration of\n" +
				"[package versioning in Go](https://research.swtch.com/vgo1).",
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name:           "not markdown readme",
			readmeFilePath: "README.rst",
			readmeContents: "This package collects pithy sayings.\n\n" +
				"It's part of a demonstration of\n" +
				"[package versioning in Go](https://research.swtch.com/vgo1).",
			want: template.HTML("<pre class=\"readme\">This package collects pithy sayings.\n\nIt&#39;s part of a demonstration of\n[package versioning in Go](https://research.swtch.com/vgo1).</pre>"),
		},
		{
			name:           "empty readme",
			readmeFilePath: "",
			readmeContents: "",
			want:           template.HTML("<pre class=\"readme\"></pre>"),
		},
		{
			name:           "sanitized readme",
			readmeFilePath: "README",
			readmeContents: `<a onblur="alert(secret)" href="http://www.google.com">Google</a>`,
			want:           template.HTML(`<pre class="readme">&lt;a onblur=&#34;alert(secret)&#34; href=&#34;http://www.google.com&#34;&gt;Google&lt;/a&gt;</pre>`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML(tc.readmeFilePath, []byte(tc.readmeContents))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%q, %q) mismatch (-want +got):\n%s", tc.readmeFilePath, tc.readmeContents, diff)
			}
		})
	}
}

func TestFetchModuleDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *ModuleDetails
	}{
		name:    "want expected module details",
		version: postgres.SampleVersion(),
		wantDetails: &ModuleDetails{
			ModulePath: postgres.SampleModulePath,
			Version:    postgres.SampleVersionString,
			Packages:   []*Package{samplePackage},
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, postgres.SampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", tc.version, postgres.SampleLicenses, err)
	}

	got, err := fetchModuleDetails(ctx, testDB, firstVersionedPackage(tc.version))
	if err != nil {
		t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetails)
	}

	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestCreatePackageHeader(t *testing.T) {
	versionInfo := internal.VersionInfo{
		Version: "v1.0.0",
	}
	for _, tc := range []struct {
		pkg     *internal.VersionedPackage
		wantPkg *Package
	}{
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "foo",
					Path: "pa.th/to/foo",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Package foo",
				Path:       "pa.th/to/foo",
				CommitTime: "Jan  1, 0001",
				IsCommand:  false,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo/v2",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo/v2",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo/v1",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo/v1",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
	} {

		t.Run(tc.pkg.Path, func(t *testing.T) {
			got, err := createPackageHeader(tc.pkg)
			if err != nil {
				t.Fatalf("createPackageHeader(%v): %v", tc.pkg, err)
			}

			if diff := cmp.Diff(tc.wantPkg, got); diff != "" {
				t.Errorf("createPackageHeader(%v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}

		})
	}
}

func TestFetchImportsDetails(t *testing.T) {
	for _, tc := range []struct {
		name        string
		imports     []string
		wantDetails *ImportsDetails
	}{
		{
			name: "want imports details with standard and internal",
			imports: []string{
				"pa.th/import/1",
				postgres.SamplePackage.Path,
				"context",
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1"},
				InternalImports: []string{postgres.SamplePackage.Path},
				StdLib:          []string{"context"},
			},
		},
		{
			name:    "want expected imports details with multiple",
			imports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
				StdLib:          nil,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			version := postgres.SampleVersion(func(v *internal.Version) {
				pkg := postgres.SamplePackage
				pkg.Imports = tc.imports
				v.Packages = []*internal.Package{pkg}
			})
			if err := testDB.InsertVersion(ctx, version, postgres.SampleLicenses); err != nil {
				t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", version, postgres.SampleLicenses, err)
			}

			got, err := fetchImportsDetails(ctx, testDB, firstVersionedPackage(version))
			if err != nil {
				t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					version.Packages[0].Path, version.Version, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = version.VersionInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", version.Packages[0].Path, version.Version, diff)
			}
		})
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	var versionCount = 0
	makeVersion := func(packages ...*internal.Package) *internal.Version {
		v := postgres.SampleVersion()
		v.Packages = packages
		// Set Version to something unique.
		v.Version = fmt.Sprintf("v1.0.%d", versionCount)
		versionCount++
		return v
	}
	var (
		pkg1 = &internal.Package{
			Name: "bar",
			Path: "path.to/foo/bar",
		}
		pkg2 = &internal.Package{
			Name:    "bar2",
			Path:    "path2.to/foo/bar2",
			Imports: []string{pkg1.Path},
		}
		pkg3 = &internal.Package{
			Name:    "bar3",
			Path:    "path3.to/foo/bar3",
			Imports: []string{pkg2.Path, pkg1.Path},
		}
	)

	for _, tc := range []struct {
		pkg         *internal.Package
		wantDetails *ImportedByDetails
	}{
		{
			pkg:         pkg3,
			wantDetails: &ImportedByDetails{},
		},
		{
			pkg: pkg2,
			wantDetails: &ImportedByDetails{
				ExternalImportedBy: []string{pkg3.Path},
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ExternalImportedBy: []string{pkg2.Path, pkg3.Path},
			},
		},
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, p := range []*internal.Package{pkg1, pkg2, pkg3} {
				v := makeVersion(p)
				if err := testDB.InsertVersion(ctx, v, postgres.SampleLicenses); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			vp := firstVersionedPackage(makeVersion(tc.pkg))
			got, err := fetchImportedByDetails(ctx, testDB, vp)
			if err != nil {
				t.Fatalf("fetchImportedByDetails(ctx, db, %q) = %v err = %v, want %v",
					tc.pkg.Path, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = vp.VersionInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got, cmpopts.IgnoreFields(DetailsPage{}, "PackageHeader")); diff != "" {
				t.Errorf("fetchImportedByDetails(ctx, db, %q) mismatch (-want +got):\n%s", tc.pkg.Path, diff)
			}
		})
	}
}

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		wantModule  string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid_url",
			url:         "https://discovery.com/test.module@v1.0.0",
			wantModule:  "test.module",
			wantVersion: "v1.0.0",
		},
		{
			name:        "valid_url_with_tab",
			url:         "https://discovery.com/test.module@v1.0.0?tab=docs",
			wantModule:  "test.module",
			wantVersion: "v1.0.0",
		},
		{
			name:        "valid_url_missing_version",
			url:         "https://discovery.com/module",
			wantModule:  "module",
			wantVersion: "",
		},
		{
			name:    "invalid_url",
			url:     "https://discovery.com/",
			wantErr: true,
		},
		{
			name:    "invalid_url_missing_module",
			url:     "https://discovery.com@v1.0.0",
			wantErr: true,
		},
		{
			name:        "invalid_version",
			url:         "https://discovery.com/module@v1.0.0invalid",
			wantModule:  "module",
			wantVersion: "v1.0.0invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			gotModule, gotVersion, err := parseModulePathAndVersion(u.Path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want error %t)", u, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModule != gotModule || tc.wantVersion != gotVersion) {
				t.Fatalf("parseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, want err %t",
					u, gotModule, gotVersion, err, tc.wantModule, tc.wantVersion, tc.wantErr)
			}
		})
	}
}
