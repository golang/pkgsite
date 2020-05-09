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
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

func cleanFetchResult(fr *FetchResult, detector *licenses.Detector) *FetchResult {
	fr.ModulePath = fr.Module.ModulePath
	if fr.GoModPath == "" && fr.ModulePath != stdlib.ModulePath {
		fr.GoModPath = fr.ModulePath
	}
	if fr.Status == 0 {
		fr.Status = 200
	}
	if fr.Module.Version == "" {
		fr.Module.Version = "v1.0.0"
	}
	fr.RequestedVersion = fr.Module.Version
	fr.ResolvedVersion = fr.Module.Version
	if fr.Module.VersionType == "" {
		fr.Module.VersionType = version.TypeRelease
	}
	if fr.Module.CommitTime.IsZero() {
		fr.Module.CommitTime = testProxyCommitTime
	}

	allLicenses := detector.AllLicenses()
	if len(allLicenses) > 0 {
		fr.Module.Licenses = allLicenses
		fr.Module.IsRedistributable = true
		for _, d := range fr.Module.Directories {
			isRedist, lics := detector.PackageInfo(
				strings.TrimPrefix(strings.TrimPrefix(d.Path, fr.ModulePath), "/"))
			for _, l := range lics {
				d.Licenses = append(d.Licenses, l.Metadata)
			}
			d.IsRedistributable = isRedist
		}
	}

	shouldSetPVS := (fr.PackageVersionStates == nil)
	for _, dir := range fr.Module.Directories {
		if dir.Package != nil {
			if dir.Package.Documentation.GOOS == "" {
				dir.Package.Documentation.GOOS = "linux"
				dir.Package.Documentation.GOARCH = "amd64"
			}
			dir.Package.Path = dir.Path
			fr.Module.Packages = append(fr.Module.Packages, &internal.Package{
				Path:              dir.Path,
				V1Path:            dir.V1Path,
				Licenses:          dir.Licenses,
				Name:              dir.Package.Name,
				Synopsis:          dir.Package.Documentation.Synopsis,
				DocumentationHTML: dir.Package.Documentation.HTML,
				Imports:           dir.Package.Imports,
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
		zipReader, _, err = stdlib.Zip(version)
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
	sort.Slice(fr.Module.Packages, func(i, j int) bool {
		return fr.Module.Packages[i].Path < fr.Module.Packages[j].Path
	})
	sort.Slice(fr.Module.Directories, func(i, j int) bool {
		return fr.Module.Directories[i].Path < fr.Module.Directories[j].Path
	})
	sort.Slice(fr.Module.Licenses, func(i, j int) bool {
		return fr.Module.Licenses[i].FilePath < fr.Module.Licenses[j].FilePath
	})
	sort.Slice(fr.PackageVersionStates, func(i, j int) bool {
		return fr.PackageVersionStates[i].PackagePath < fr.PackageVersionStates[j].PackagePath
	})
	for _, dir := range fr.Module.Directories {
		sort.Slice(dir.Licenses, func(i, j int) bool {
			return dir.Licenses[i].FilePath < dir.Licenses[j].FilePath
		})
	}
	for _, pkg := range fr.Module.Packages {
		sort.Slice(pkg.Licenses, func(i, j int) bool {
			return pkg.Licenses[i].FilePath < pkg.Licenses[j].FilePath
		})
	}
}

func validateDocumentationHTML(t *testing.T, got, want *internal.Module) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Recovered in checkDocumentationHTML: %v \n; diff = %s", r, cmp.Diff(want, got))
		}
	}()
	for i := 0; i < len(want.Packages); i++ {
		wantHTML := want.Packages[i].DocumentationHTML
		gotHTML := got.Packages[i].DocumentationHTML
		if len(wantHTML) != 0 && !strings.Contains(gotHTML, wantHTML) {
			t.Errorf("documentation for got.Module.Packages[%d].DocumentationHTML does not contain wanted documentation substring:\n want (substring): %q\n got: %q\n", i, wantHTML, gotHTML)
		}
	}
	for i := 0; i < len(want.Directories); i++ {
		if want.Directories[i].Package == nil {
			continue
		}
		wantHTML := want.Directories[i].Package.Documentation.HTML
		gotHTML := got.Directories[i].Package.Documentation.HTML
		if len(wantHTML) != 0 && !strings.Contains(gotHTML, wantHTML) {
			t.Errorf("documentation for got.Module.Directories[%d].DocumentationHTML does not contain wanted documentation substring:\n want (substring): %q\n got: %q\n", i, wantHTML, gotHTML)
		}
	}
}
