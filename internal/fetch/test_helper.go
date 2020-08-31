// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/stdlib"
)

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

func cleanFetchResult(fr *FetchResult, detector *licenses.Detector) *FetchResult {
	fr.ModulePath = fr.Module.ModulePath
	if fr.GoModPath == "" {
		fr.GoModPath = fr.ModulePath
	}
	if fr.Status == 0 {
		fr.Status = 200
	}
	if fr.Module.Version == "" {
		fr.Module.Version = "v1.0.0"
	}
	if fr.RequestedVersion == "" {
		fr.RequestedVersion = fr.Module.Version
	}
	fr.ResolvedVersion = fr.Module.Version
	if fr.Module.CommitTime.IsZero() {
		fr.Module.CommitTime = testProxyCommitTime
	}

	allLicenses := detector.AllLicenses()
	if len(allLicenses) > 0 {
		fr.Module.Licenses = allLicenses
		fr.Module.IsRedistributable = true
		for _, d := range fr.Module.Units {
			isRedist, lics := detector.PackageInfo(internal.Suffix(d.Path, fr.ModulePath))
			for _, l := range lics {
				d.Licenses = append(d.Licenses, l.Metadata)
			}
			d.IsRedistributable = isRedist
		}
	}

	shouldSetPVS := (fr.PackageVersionStates == nil)
	for _, dir := range fr.Module.Units {
		if dir.Package != nil {
			if dir.Package.Documentation.GOOS == "" {
				dir.Package.Documentation.GOOS = "linux"
				dir.Package.Documentation.GOARCH = "amd64"
			}
			dir.Package.Path = dir.Path
			fr.Module.LegacyPackages = append(fr.Module.LegacyPackages, &internal.LegacyPackage{
				Path:              dir.Path,
				V1Path:            dir.V1Path,
				Licenses:          dir.Licenses,
				Name:              dir.Package.Name,
				Synopsis:          dir.Package.Documentation.Synopsis,
				DocumentationHTML: dir.Package.Documentation.HTML,
				Imports:           dir.Imports,
				GOOS:              dir.Package.Documentation.GOOS,
				GOARCH:            dir.Package.Documentation.GOARCH,
				IsRedistributable: dir.IsRedistributable,
			})
			if shouldSetPVS {
				fr.PackageVersionStates = append(
					fr.PackageVersionStates, &internal.PackageVersionState{
						PackagePath: dir.Path,
						ModulePath:  fr.Module.ModulePath,
						Version:     fr.Module.Version,
						Status:      http.StatusOK,
					},
				)
			}
		}
	}
	return fr
}

func licenseDetector(ctx context.Context, t *testing.T, modulePath, version string, proxyClient *proxy.Client) *licenses.Detector {
	t.Helper()
	var (
		zipReader *zip.Reader
		err       error
	)
	if modulePath == stdlib.ModulePath {
		zipReader, _, _, err = stdlib.Zip(version)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		zipReader, err = proxyClient.GetZip(ctx, modulePath, version)
		if err != nil {
			t.Fatal(err)
		}
	}
	logf := func(format string, args ...interface{}) {
		log.Infof(ctx, format, args...)
	}
	return licenses.NewDetector(modulePath, version, zipReader, logf)
}

func sortFetchResult(fr *FetchResult) {
	sort.Slice(fr.Module.LegacyPackages, func(i, j int) bool {
		return fr.Module.LegacyPackages[i].Path < fr.Module.LegacyPackages[j].Path
	})
	sort.Slice(fr.Module.Units, func(i, j int) bool {
		return fr.Module.Units[i].Path < fr.Module.Units[j].Path
	})
	sort.Slice(fr.Module.Licenses, func(i, j int) bool {
		return fr.Module.Licenses[i].FilePath < fr.Module.Licenses[j].FilePath
	})
	sort.Slice(fr.PackageVersionStates, func(i, j int) bool {
		return fr.PackageVersionStates[i].PackagePath < fr.PackageVersionStates[j].PackagePath
	})
	for _, dir := range fr.Module.Units {
		sort.Slice(dir.Licenses, func(i, j int) bool {
			return dir.Licenses[i].FilePath < dir.Licenses[j].FilePath
		})
	}
	for _, pkg := range fr.Module.LegacyPackages {
		sort.Slice(pkg.Licenses, func(i, j int) bool {
			return pkg.Licenses[i].FilePath < pkg.Licenses[j].FilePath
		})
	}
}

// validateDocumentationHTML checks that the doc HTML contains a set of
// substrings. The desired strings consist of the "want" module's documentation
// fields, with multiple substrings separated by '~'.
func validateDocumentationHTML(t *testing.T, got, want *internal.Module) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Recovered in checkDocumentationHTML: %v \n; diff = %s", r, cmp.Diff(want, got))
		}
	}()

	checkHTML := func(msg string, i int, hGot, hWant safehtml.HTML) {
		t.Helper()
		got := hGot.String()
		// Treat the wanted DocumentationHTML as a set of substrings to look for, separated by ~.
		for _, want := range strings.Split(hWant.String(), "~") {
			want = strings.TrimSpace(want)
			if !strings.Contains(got, want) {
				t.Errorf("doc for %s[%d]:\nmissing %q; got\n%q", msg, i, want, got)
			}
		}
	}

	for i := 0; i < len(want.LegacyPackages); i++ {
		checkHTML("LegacyPackages", i, got.LegacyPackages[i].DocumentationHTML, want.LegacyPackages[i].DocumentationHTML)
	}
	for i := 0; i < len(want.Units); i++ {
		if want.Units[i].Package == nil {
			continue
		}
		checkHTML("Directories", i, got.Units[i].Package.Documentation.HTML, want.Units[i].Package.Documentation.HTML)
	}
}
