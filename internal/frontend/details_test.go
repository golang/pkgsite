// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
)

func samplePackage(mutators ...func(*Package)) *Package {
	p := &Package{
		Version:           sample.VersionString,
		Path:              sample.PackagePath,
		CommitTime:        "0 hours ago",
		Suffix:            sample.PackageName,
		ModulePath:        sample.ModulePath,
		RepositoryURL:     sample.RepositoryURL,
		Synopsis:          sample.Synopsis,
		IsRedistributable: true,
		Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
	}
	for _, mut := range mutators {
		mut(p)
	}
	return p
}

func TestElapsedTime(t *testing.T) {
	now := sample.NowTruncated()
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

func TestFetchReadMeDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *ReadMeDetails
	}{
		name:    "want expected overview details",
		version: sample.Version(),
		wantDetails: &ReadMeDetails{
			ModulePath: sample.ModulePath,
			ReadMe:     template.HTML("<p>readme</p>\n"),
		},
	}

	defer postgres.ResetTestDB(testDB, t)

	if err := testDB.InsertVersion(ctx, tc.version, sample.Licenses); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchReadMeDetails(ctx, testDB, firstVersionedPackage(tc.version))
	if err != nil {
		t.Fatalf("fetchReadMeDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetails)
	}

	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchReadMeDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name, readmeFilePath, readmeContents, repositoryURL string
		want                                                template.HTML
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
		{
			name:           "relative image markdown is made absolute for GitHub",
			readmeFilePath: "README.md",
			readmeContents: "![Go logo](doc/logo.png)",
			repositoryURL:  "http://github.com/golang/go",
			want:           template.HTML("<p><img src=\"https://raw.githubusercontent.com/golang/go/master/doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name:           "relative image markdown is left alone for unknown origins",
			readmeFilePath: "README.md",
			readmeContents: "![Go logo](doc/logo.png)",
			repositoryURL:  "http://example.com/golang/go",
			want:           template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
		{
			name:           "valid markdown readme, invalid repositoryURL",
			readmeFilePath: "README.md",
			readmeContents: "![Go logo](doc/logo.png)",
			repositoryURL:  ":",
			want:           template.HTML("<p><img src=\"doc/logo.png\" alt=\"Go logo\"/></p>\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML(tc.readmeFilePath, []byte(tc.readmeContents), tc.repositoryURL)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%q, %q) mismatch (-want +got):\n%s", tc.readmeFilePath, tc.readmeContents, diff)
			}
		})
	}
}

func TestFetchModuleDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *ModuleDetails
	}{
		name:    "want expected module details",
		version: sample.Version(),
		wantDetails: &ModuleDetails{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Packages:   []*Package{samplePackage()},
		},
	}

	if err := testDB.InsertVersion(ctx, tc.version, sample.Licenses); err != nil {
		t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", tc.version, sample.Licenses, err)
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
	for _, tc := range []struct {
		label   string
		pkg     *internal.VersionedPackage
		wantPkg *Package
	}{
		{
			label:   "simple package",
			pkg:     sample.VersionedPackage(),
			wantPkg: samplePackage(),
		},
		{
			label: "command package",
			pkg: sample.VersionedPackage(func(vp *internal.VersionedPackage) {
				vp.Name = "main"
			}),
			wantPkg: samplePackage(),
		},
		{
			label: "v2 command",
			pkg: sample.VersionedPackage(func(vp *internal.VersionedPackage) {
				vp.Name = "main"
				vp.Path = "pa.th/to/foo/v2/bar"
				vp.ModulePath = "pa.th/to/foo/v2"
			}),
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v2/bar"
				p.Suffix = "bar"
				p.ModulePath = "pa.th/to/foo/v2"
			}),
		},
		{
			label: "explicit v1 command",
			pkg: sample.VersionedPackage(func(vp *internal.VersionedPackage) {
				vp.Name = "main"
				vp.Path = "pa.th/to/foo/v1"
				vp.ModulePath = "pa.th/to/foo/v1"
			}),
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v1"
				p.Suffix = "foo (root)"
				p.ModulePath = "pa.th/to/foo/v1"
			}),
		},
	} {

		t.Run(tc.label, func(t *testing.T) {
			got, err := createPackage(&tc.pkg.Package, &tc.pkg.VersionInfo)
			if err != nil {
				t.Fatalf("createPackage(%v): %v", tc.pkg, err)
			}

			if diff := cmp.Diff(tc.wantPkg, got); diff != "" {
				t.Errorf("createPackage(%v) mismatch (-want +got):\n%s", tc.pkg, diff)
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
				sample.PackagePath,
				"context",
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1"},
				InternalImports: []string{sample.PackagePath},
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

			version := sample.Version(func(v *internal.Version) {
				pkg := sample.Package()
				pkg.Imports = tc.imports
				v.Packages = []*internal.Package{pkg}
			})
			if err := testDB.InsertVersion(ctx, version, sample.Licenses); err != nil {
				t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", version, sample.Licenses, err)
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
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		pkg1    = sample.Package(sample.WithPath("path.to/foo/bar"))
		pkg2    = sample.Package(sample.WithPath("path2.to/foo/bar2"), sample.WithImports(pkg1.Path))
		pkg3    = sample.Package(sample.WithPath("path3.to/foo/bar3"), sample.WithImports(pkg2.Path, pkg1.Path))
		sampler = sample.VersionSampler(func() *internal.Version {
			return sample.Version(sample.WithModulePath("path.to/foo"))
		})
		testVersions = []*internal.Version{
			sampler.Sample(sample.WithPackages(pkg1)),
			sampler.Sample(sample.WithModulePath("path2.to/foo"), sample.WithPackages(pkg2)),
			sampler.Sample(sample.WithModulePath("path3.to/foo"), sample.WithPackages(pkg3)),
		}
	)

	for _, v := range testVersions {
		if err := testDB.InsertVersion(ctx, v, sample.Licenses); err != nil {
			t.Fatalf("db.InsertVersion(%v): %v", v, err)
		}
	}

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
				ImportedBy: []string{pkg3.Path},
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ImportedBy: []string{pkg2.Path, pkg3.Path},
			},
		},
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			otherVersion := sampler.Sample(sample.WithVersion("v1.0.5"), sample.WithPackages(tc.pkg))
			vp := firstVersionedPackage(otherVersion)
			got, err := fetchImportedByDetails(ctx, testDB, vp, paginationParams{limit: 20, page: 1})
			if err != nil {
				t.Fatalf("fetchImportedByDetails(ctx, db, %q, 1) = %v err = %v, want %v",
					tc.pkg.Path, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = vp.VersionInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got, cmpopts.IgnoreFields(ImportedByDetails{}, "Pagination")); diff != "" {
				t.Errorf("fetchImportedByDetails(ctx, db, %q, 1) mismatch (-want +got):\n%s", tc.pkg.Path, diff)
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
