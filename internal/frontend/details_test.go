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
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/postgres"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

var (
	sampleLicenseMetadata = []*license.Metadata{
		{Type: "MIT", FilePath: "LICENSE"},
	}
	sampleLicenses = []*license.License{
		{Metadata: *sampleLicenseMetadata[0], Contents: []byte("Lorem Ipsum")},
	}
	sampleInternalPackage = &internal.Package{
		Name:     "pkg_name",
		Path:     "test.com/module/pkg_name",
		Synopsis: "Test package synopsis",
		Licenses: sampleLicenseMetadata,
	}
	sampleInternalVersion = &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:     "series",
			ModulePath:     "test.com/module",
			Version:        "v1.0.0",
			CommitTime:     time.Now().Add(time.Hour * -8),
			ReadmeFilePath: "README.md",
			ReadmeContents: []byte("This is the readme text."),
			VersionType:    internal.VersionTypeRelease,
		},
		Packages: []*internal.Package{sampleInternalPackage},
	}
	samplePackage = &Package{
		Name:       "pkg_name",
		ModulePath: "test.com/module",
		Version:    "v1.0.0",
		Path:       "test.com/module/pkg_name",
		Suffix:     "pkg_name",
		Synopsis:   "Test package synopsis",
		Licenses:   transformLicenseMetadata(sampleLicenseMetadata),
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
		version: sampleInternalVersion,
		wantDetails: &OverviewDetails{
			ModulePath: "test.com/module",
			ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, sampleLicenses); err != nil {
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

func getTestVersion(pkgPath, modulePath, version string, versionType internal.VersionType, packages ...*internal.Package) *internal.Version {
	return &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:     "test.com/module",
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     time.Now().Add(time.Hour * -8),
			ReadmeContents: []byte("This is the readme text."),
			VersionType:    versionType,
		},
		Packages: packages,
	}
}

func TestFetchVersionsDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		modulePath1 = "test.com/module"
		modulePath2 = "test.com/module/v2"
		pkg1Path    = "test.com/module/pkg_name"
		pkg2Path    = "test.com/module/v2/pkg_name"
		versionInfo = internal.VersionInfo{
			SeriesPath: "test.com/module",
			ModulePath: "test.com/module/v2",
			Version:    "v2.2.1-alpha.1",
		}
		pkg1 = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "pkg_name",
				Path:     pkg1Path,
				Synopsis: "test synopsis",
				Licenses: sampleLicenseMetadata,
				Suffix:   "pkg_name",
			},
			VersionInfo: versionInfo,
		}
		latestTagged = &Package{
			Version:    "2.2.1-alpha.1",
			Path:       pkg2Path,
			CommitTime: "today",
		}
		latestPseudo = &Package{
			Version:    "0.0.0-20140414041502-3c2ca4d52544",
			Path:       pkg1Path,
			CommitTime: "today",
		}
	)

	for _, tc := range []struct {
		name, path, version string
		versions            []*internal.Version
		wantDetails         *VersionsDetails
	}{
		{
			name:    "want tagged versions",
			path:    pkg1Path,
			version: "v1.2.1",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, modulePath1, "v1.2.3", internal.VersionTypeRelease, &pkg1.Package),
				getTestVersion(pkg2Path, modulePath2, "v2.0.0", internal.VersionTypeRelease, &pkg1.Package),
				getTestVersion(pkg1Path, modulePath1, "v1.3.0", internal.VersionTypeRelease, &pkg1.Package),
				getTestVersion(pkg1Path, modulePath1, "v1.2.1", internal.VersionTypeRelease, &pkg1.Package),
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg1.Package),
				getTestVersion(pkg2Path, modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease, &pkg1.Package),
			},
			wantDetails: &VersionsDetails{
				Versions: []*SeriesVersionGroup{{
					Series: "test.com/module",
					Latest: latestTagged,
					MajorVersions: []*MajorVersionGroup{{
						Major:  "v2",
						Latest: latestTagged,
						MinorVersions: []*MinorVersionGroup{{
							Minor:         "2.2",
							Latest:        latestTagged,
							PatchVersions: []*Package{latestTagged},
						}, {
							Minor: "2.0",
							Latest: &Package{
								Version:    "2.0.0",
								Path:       pkg2Path,
								CommitTime: "today",
							},
							PatchVersions: []*Package{{
								Version:    "2.0.0",
								Path:       pkg2Path,
								CommitTime: "today",
							}},
						}},
					}, {
						Major: "v1",
						Latest: &Package{
							Version:    "1.3.0",
							Path:       pkg1Path,
							CommitTime: "today",
						},
						MinorVersions: []*MinorVersionGroup{{
							Minor: "1.3",
							Latest: &Package{
								Version:    "1.3.0",
								Path:       pkg1Path,
								CommitTime: "today",
							},
							PatchVersions: []*Package{{
								Version:    "1.3.0",
								Path:       pkg1Path,
								CommitTime: "today",
							}},
						}, {
							Minor: "1.2",
							Latest: &Package{
								Version:    "1.2.3",
								Path:       pkg1Path,
								CommitTime: "today",
							},
							PatchVersions: []*Package{{
								Version:    "1.2.3",
								Path:       pkg1Path,
								CommitTime: "today",
							}, {
								Version:    "1.2.1",
								Path:       pkg1Path,
								CommitTime: "today",
							}},
						}},
					}},
				}},
			},
		}, {
			name:    "want only pseudo",
			path:    pkg1Path,
			version: "v0.0.0-20140414041501-3c2ca4d52544",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo, &pkg1.Package),
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg1.Package),
			},
			wantDetails: &VersionsDetails{
				Versions: []*SeriesVersionGroup{{
					Series: "test.com/module",
					Latest: latestPseudo,
					MajorVersions: []*MajorVersionGroup{{
						Major:  "v0",
						Latest: latestPseudo,
						MinorVersions: []*MinorVersionGroup{{
							Minor:  "0.0",
							Latest: latestPseudo,
							PatchVersions: []*Package{
								latestPseudo,
								{
									Version:    "0.0.0-20140414041501-3c2ca4d52544",
									Path:       pkg1Path,
									CommitTime: "today",
								},
							},
						}},
					}},
				}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Fatalf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := fetchVersionsDetails(ctx, testDB, pkg1)
			if err != nil {
				t.Fatalf("fetchVersionsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					tc.path, tc.version, got, err, tc.wantDetails)
			}

			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchVersionsDetails(ctx, db, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
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
		version: sampleInternalVersion,
		wantDetails: &ModuleDetails{
			ModulePath: "test.com/module",
			ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
			Version:    "v1.0.0",
			Packages:   []*Package{samplePackage},
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", tc.version, sampleLicenses, err)
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
		imports     []*internal.Import
		wantDetails *ImportsDetails
	}{
		{
			name: "want imports details with standard and internal",
			imports: []*internal.Import{
				{Name: "import1", Path: "pa.th/import/1"},
				{Name: sampleInternalVersion.Packages[0].Name, Path: sampleInternalVersion.Packages[0].Path},
				{Name: "context", Path: "context"},
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []*internal.Import{
					{
						Name: "import1",
						Path: "pa.th/import/1",
					},
				},
				InternalImports: []*internal.Import{
					{
						Name: sampleInternalVersion.Packages[0].Name,
						Path: sampleInternalVersion.Packages[0].Path,
					},
				},
				StdLib: []*internal.Import{
					{
						Name: "context",
						Path: "context",
					},
				},
			},
		},
		{
			name: "want expected imports details with multiple",
			imports: []*internal.Import{
				{Name: "import1", Path: "pa.th/import/1"},
				{Name: "import2", Path: "pa.th/import/2"},
				{Name: "import3", Path: "pa.th/import/3"},
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []*internal.Import{
					{
						Name: "import1",
						Path: "pa.th/import/1",
					},
					{
						Name: "import2",
						Path: "pa.th/import/2",
					},
					{
						Name: "import3",
						Path: "pa.th/import/3",
					},
				},
				StdLib: nil,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := sampleInternalVersion
			version.Packages[0].Imports = tc.imports

			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := testDB.InsertVersion(ctx, version, sampleLicenses); err != nil {
				t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", version, sampleLicenses, err)
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
		v := *sampleInternalVersion
		v.Packages = packages
		// Set Version to something unique.
		v.Version = fmt.Sprintf("v1.0.%d", versionCount)
		versionCount++
		return &v
	}
	var (
		pkg1 = &internal.Package{
			Name: "bar",
			Path: "path.to/foo/bar",
		}
		pkg2 = &internal.Package{
			Name: "bar2",
			Path: "path2.to/foo/bar2",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
		}
		pkg3 = &internal.Package{
			Name: "bar3",
			Path: "path3.to/foo/bar3",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg2.Name,
					Path: pkg2.Path,
				},
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
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
				ExternalImportedBy: []*internal.Import{
					{Path: pkg3.Path, Name: pkg3.Name},
				},
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ExternalImportedBy: []*internal.Import{
					{Name: pkg2.Name, Path: pkg2.Path},
					{Name: pkg3.Name, Path: pkg3.Path},
				},
			},
		},
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, p := range []*internal.Package{pkg1, pkg2, pkg3} {
				v := makeVersion(p)
				if err := testDB.InsertVersion(ctx, v, sampleLicenses); err != nil {
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
			name:    "invalid_version",
			url:     "https://discovery.com/module@v1.0.0invalid",
			wantErr: true,
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
