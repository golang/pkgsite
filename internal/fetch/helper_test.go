// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"context"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var testProxyCommitTime = time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)

// cleanFetchResult adds missing information to a given FetchResult and returns
// it. It's meant to be used with test cases in fetchdata_test and should be called
// only once for each test case. The missing information is added here to avoid
// having to hardcode it into each test case.
func cleanFetchResult(t *testing.T, fr *FetchResult) *FetchResult {
	t.Helper()

	fr.ModulePath = fr.Module.ModulePath
	if fr.GoModPath == "" {
		fr.GoModPath = fr.ModulePath
	}
	if fr.Status == 0 {
		fr.Status = 200
	}
	if fr.Module.Version == "" {
		fr.Module.Version = sample.VersionString
	}
	if fr.RequestedVersion == "" {
		fr.RequestedVersion = fr.Module.Version
	}
	fr.ResolvedVersion = fr.Module.Version
	if fr.Module.CommitTime.IsZero() {
		fr.Module.CommitTime = testProxyCommitTime
	}

	shouldSetPVS := (fr.PackageVersionStates == nil)
	for _, u := range fr.Module.Units {
		u.UnitMeta = internal.UnitMeta{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        fr.Module.ModulePath,
				Version:           fr.Module.Version,
				IsRedistributable: fr.Module.IsRedistributable,
			},
			Path:              u.Path,
			Name:              u.Name,
			IsRedistributable: u.IsRedistributable,
			Licenses:          u.Licenses,
		}
		if u.IsPackage() && shouldSetPVS {
			fr.PackageVersionStates = append(
				fr.PackageVersionStates, &internal.PackageVersionState{
					PackagePath: u.Path,
					ModulePath:  fr.Module.ModulePath,
					Version:     fr.Module.Version,
					Status:      http.StatusOK,
				},
			)
		}
		for _, d := range u.Documentation {
			for _, s := range d.API {
				s.GOOS = d.GOOS
				s.GOARCH = d.GOARCH
				for _, c := range s.Children {
					c.GOOS = d.GOOS
					c.GOARCH = d.GOARCH
				}
			}
		}
	}
	return fr
}

func cleanLicenses(t *testing.T, fr *FetchResult, detector *licenses.Detector) *FetchResult {
	fr.Module.Licenses = nil
	for _, u := range fr.Module.Units {
		u.Licenses = nil
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
	return fr
}

// updateFetchResultVersions updates units' and package version states' version
// based on the type of fetching. Should be used for test cases in fetchdata_test.
func updateFetchResultVersions(t *testing.T, fr *FetchResult, local bool) *FetchResult {
	t.Helper()

	if local {
		for _, u := range fr.Module.Units {
			u.UnitMeta.Version = LocalVersion
		}
		for _, pvs := range fr.PackageVersionStates {
			pvs.Version = LocalVersion
		}
	} else {
		for _, u := range fr.Module.Units {
			u.UnitMeta.Version = fr.Module.Version
		}
		for _, pvs := range fr.PackageVersionStates {
			pvs.Version = fr.Module.Version
		}
	}
	return fr
}

// proxyFetcher is a test helper function that sets up a test proxy, fetches
// a module using FetchModule, and returns fetch result and a license detector.
func proxyFetcher(t *testing.T, withLicenseDetector bool, ctx context.Context, mod *proxy.Module, fetchVersion string) (*FetchResult, *licenses.Detector) {
	t.Helper()

	modulePath := mod.ModulePath
	version := mod.Version
	if version == "" {
		version = sample.VersionString
	}
	if fetchVersion == "" {
		fetchVersion = version
	}

	proxyClient, teardownProxy := proxy.SetupTestClient(t, []*proxy.Module{{
		ModulePath: modulePath,
		Version:    version,
		Files:      mod.Files,
	}})
	defer teardownProxy()
	got := FetchModule(ctx, modulePath, fetchVersion, proxyClient, source.NewClientForTesting())
	if !withLicenseDetector {
		return got, nil
	}

	d := licenseDetector(ctx, t, modulePath, got.ResolvedVersion, proxyClient)
	return got, d
}

// localFetcher is a helper function that creates a test directory to hold a module,
// fetches the module from the directory using FetchLocalModule, and returns a fetch
// result, and a license detector.
func localFetcher(t *testing.T, withLicenseDetector bool, ctx context.Context, mod *proxy.Module, fetchVersion string) (*FetchResult, *licenses.Detector) {
	t.Helper()

	directory, err := testhelper.CreateTestDirectory(mod.Files)
	if err != nil {
		t.Fatalf("couldn't create test files")
	}
	defer os.RemoveAll(directory)

	modulePath := mod.ModulePath
	got := FetchLocalModule(ctx, modulePath, directory, source.NewClientForTesting())
	if !withLicenseDetector {
		return got, nil
	}

	zipReader, err := createZipReader(directory, modulePath, LocalVersion)
	if err != nil {
		t.Fatal("couldn't create zip reader")
	}
	d := licenses.NewDetector(modulePath, LocalVersion, zipReader, func(format string, args ...interface{}) {
		log.Infof(ctx, format, args...)
	})
	return got, d
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
		zipReader, err = proxyClient.Zip(ctx, modulePath, version)
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
}
