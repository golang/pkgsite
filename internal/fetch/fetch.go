// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

var (
	ErrModuleContainsNoPackages = errors.New("module contains 0 packages")
	errMalformedZip             = errors.New("module zip is malformed")
)

var (
	fetchLatency = stats.Float64(
		"go-discovery/worker/fetch-latency",
		"Latency of a fetch request.",
		stats.UnitSeconds,
	)
	fetchesShedded = stats.Int64(
		"go-discovery/worker/fetch-shedded",
		"Count of shedded fetches.",
		stats.UnitDimensionless,
	)
	fetchedPackages = stats.Int64(
		"go-discovery/worker/fetch-package-count",
		"Count of successfully fetched packages.",
		stats.UnitDimensionless,
	)

	// FetchLatencyDistribution aggregates frontend fetch request
	// latency by status code. It does not count shedded requests.
	FetchLatencyDistribution = &view.View{
		Name:        "go-discovery/worker/fetch-latency",
		Measure:     fetchLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Fetch latency by result status.",
		TagKeys:     []tag.Key{dcensus.KeyStatus},
	}
	// FetchResponseCount counts fetch responses by status.
	FetchResponseCount = &view.View{
		Name:        "go-discovery/worker/fetch-count",
		Measure:     fetchLatency,
		Aggregation: view.Count(),
		Description: "Fetch request count by result status",
		TagKeys:     []tag.Key{dcensus.KeyStatus},
	}
	// FetchPackageCount counts how many packages were successfully fetched.
	FetchPackageCount = &view.View{
		Name:        "go-discovery/worker/fetch-package-count",
		Measure:     fetchedPackages,
		Aggregation: view.Count(),
		Description: "Count of packages successfully fetched",
	}
	// SheddedFetchCount counts the number of fetches that were shedded.
	SheddedFetchCount = &view.View{
		Name:        "go-discovery/worker/fetch-shedded",
		Measure:     fetchesShedded,
		Aggregation: view.Count(),
		Description: "Count of shedded fetches",
	}
)

type FetchResult struct {
	ModulePath           string
	RequestedVersion     string
	ResolvedVersion      string
	MainVersion          string
	MasterVersion        string
	GoModPath            string
	Status               int
	Error                error
	Defer                func() // caller must defer this on all code paths
	Module               *internal.Module
	PackageVersionStates []*internal.PackageVersionState
}

// FetchModule queries the proxy or the Go repo for the requested module
// version, downloads the module zip, and processes the contents to return an
// *internal.Module and related information.
//
// Even if err is non-nil, the result may contain useful information, like the go.mod path.
//
// Callers of FetchModule must
//   defer fr.Defer()
// immediately after the call.
func FetchModule(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, disableProxyFetch bool) (fr *FetchResult) {
	start := time.Now()
	fr = &FetchResult{
		ModulePath:       modulePath,
		RequestedVersion: requestedVersion,
		Defer:            func() {},
	}
	var fi *FetchInfo
	defer func() {
		if fr.Error != nil {
			derrors.Wrap(&fr.Error, "FetchModule(%q, %q)", modulePath, requestedVersion)
			fr.Status = derrors.ToStatus(fr.Error)
		}
		if fr.Status == 0 {
			fr.Status = http.StatusOK
		}
		latency := float64(time.Since(start).Seconds())
		dcensus.RecordWithTag(ctx, dcensus.KeyStatus, strconv.Itoa(fr.Status), fetchLatency.M(latency))
		if fr.Status < 300 {
			stats.Record(ctx, fetchedPackages.M(int64(len(fr.PackageVersionStates))))
		}
		if fi != nil {
			finishFetchInfo(fi, fr.Status, fr.Error)
		}
	}()

	var commitTime time.Time
	info, err := GetInfo(ctx, modulePath, requestedVersion, proxyClient, disableProxyFetch)
	if err != nil {
		fr.Error = err
		return fr
	}
	fr.ResolvedVersion = info.Version
	commitTime = info.Time

	var zipSize int64
	if zipLoadShedder != nil {
		zipSize, err := getZipSize(ctx, modulePath, fr.ResolvedVersion, proxyClient)
		if err != nil {
			fr.Error = err
			return fr
		}
		// Load shed or mark module as too large.
		// We treat zip size as a proxy for the total memory consumed by
		// processing a module, and use it to decide whether we can currently
		// afford to process a module.
		shouldShed, deferFunc := zipLoadShedder.shouldShed(uint64(zipSize))
		fr.Defer = deferFunc
		if shouldShed {
			fr.Error = fmt.Errorf("%w: size=%dMi", derrors.SheddingLoad, zipSize/mib)
			stats.Record(ctx, fetchesShedded.M(1))
			return fr
		}
		if zipSize > maxModuleZipSize {
			log.Warningf(ctx, "FetchModule: %s@%s zip size %dMi exceeds max %dMi",
				modulePath, fr.ResolvedVersion, zipSize/mib, maxModuleZipSize/mib)
			fr.Error = derrors.ModuleTooLarge
			return fr
		}
	}

	// Proceed with the fetch.
	fi = &FetchInfo{
		ModulePath: modulePath,
		Version:    fr.ResolvedVersion,
		ZipSize:    uint64(zipSize),
		Start:      time.Now(),
	}
	startFetchInfo(fi)

	var zipReader *zip.Reader
	if modulePath == stdlib.ModulePath {
		zipReader, commitTime, err = stdlib.Zip(requestedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		fr.GoModPath = stdlib.ModulePath
	} else {
		goModBytes, err := proxyClient.GetMod(ctx, modulePath, fr.ResolvedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
		goModPath := modfile.ModulePath(goModBytes)
		if goModPath == "" {
			fr.Error = fmt.Errorf("go.mod has no module path: %w", derrors.BadModule)
			return fr
		}
		fr.GoModPath = goModPath
		if goModPath != modulePath {
			// The module path in the go.mod file doesn't match the path of the
			// zip file. Don't insert the module. Store an AlternativeModule
			// status in module_version_states.
			fr.Error = fmt.Errorf("module path=%s, go.mod path=%s: %w", modulePath, goModPath, derrors.AlternativeModule)
			return fr
		}
		zipReader, err = proxyClient.GetZip(ctx, modulePath, fr.ResolvedVersion)
		if err != nil {
			fr.Error = err
			return fr
		}
	}
	mod, pvs, err := processZipFile(ctx, modulePath, fr.ResolvedVersion, commitTime, zipReader, sourceClient)
	if err != nil {
		fr.Error = err
		return fr
	}
	fr.Module = mod
	fr.PackageVersionStates = pvs
	if modulePath == stdlib.ModulePath {
		fr.Module.HasGoMod = true
	}
	for _, state := range fr.PackageVersionStates {
		if state.Status != http.StatusOK {
			fr.Status = derrors.ToStatus(derrors.HasIncompletePackages)
		}
	}
	return fr
}

// GetInfo returns the result of a request to the proxy .info endpoint. If
// the modulePath is "std", a request to @master will return an empty
// commit time.
func GetInfo(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, disableProxyFetch bool) (_ *proxy.VersionInfo, err error) {
	if modulePath == stdlib.ModulePath {
		var resolvedVersion string
		resolvedVersion, err = stdlib.ZipInfo(requestedVersion)
		if err != nil {
			return nil, err
		}
		return &proxy.VersionInfo{Version: resolvedVersion}, nil
	}
	getInfo := proxyClient.GetInfo
	if disableProxyFetch {
		getInfo = proxyClient.GetInfoNoFetch
	}
	return getInfo(ctx, modulePath, requestedVersion)
}

func getZipSize(ctx context.Context, modulePath, resolvedVersion string, proxyClient *proxy.Client) (_ int64, err error) {
	if modulePath == stdlib.ModulePath {
		return stdlib.EstimatedZipSize, nil
	}
	return proxyClient.GetZipSize(ctx, modulePath, resolvedVersion)
}

// processZipFile extracts information from the module version zip.
func processZipFile(ctx context.Context, modulePath string, resolvedVersion string, commitTime time.Time, zipReader *zip.Reader, sourceClient *source.Client) (_ *internal.Module, _ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "processZipFile(%q, %q)", modulePath, resolvedVersion)

	ctx, span := trace.StartSpan(ctx, "fetch.processZipFile")
	defer span.End()

	sourceInfo, err := source.ModuleInfo(ctx, sourceClient, modulePath, resolvedVersion)
	if err != nil {
		log.Infof(ctx, "error getting source info: %v", err)
	}
	readmes, err := extractReadmesFromZip(modulePath, resolvedVersion, zipReader)
	if err != nil {
		return nil, nil, fmt.Errorf("extractReadmesFromZip(%q, %q, zipReader): %v", modulePath, resolvedVersion, err)
	}
	logf := func(format string, args ...interface{}) {
		log.Infof(ctx, format, args...)
	}
	d := licenses.NewDetector(modulePath, resolvedVersion, zipReader, logf)
	allLicenses := d.AllLicenses()
	packages, packageVersionStates, err := extractPackagesFromZip(ctx, modulePath, resolvedVersion, zipReader, d, sourceInfo)
	if errors.Is(err, ErrModuleContainsNoPackages) || errors.Is(err, errMalformedZip) {
		return nil, nil, fmt.Errorf("%v: %w", err.Error(), derrors.BadModule)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("extractPackagesFromZip(%q, %q, zipReader, %v): %v", modulePath, resolvedVersion, allLicenses, err)
	}
	hasGoMod := zipContainsFilename(zipReader, path.Join(moduleVersionDir(modulePath, resolvedVersion), "go.mod"))

	return &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:        modulePath,
			Version:           resolvedVersion,
			CommitTime:        commitTime,
			IsRedistributable: d.ModuleIsRedistributable(),
			HasGoMod:          hasGoMod,
			SourceInfo:        sourceInfo,
		},
		Licenses: allLicenses,
		Units:    moduleUnits(modulePath, resolvedVersion, packages, readmes, d),
	}, packageVersionStates, nil
}

// moduleVersionDir formats the content subdirectory for the given
// modulePath and version.
func moduleVersionDir(modulePath, version string) string {
	return fmt.Sprintf("%s@%s", modulePath, version)
}

// zipContainsFilename reports whether there is a file with the given name in the zip.
func zipContainsFilename(r *zip.Reader, name string) bool {
	for _, f := range r.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

type FetchInfo struct {
	ModulePath string
	Version    string
	ZipSize    uint64
	Start      time.Time
	Finish     time.Time
	Status     int
	Error      error
}

var (
	fetchInfoMu  sync.Mutex
	fetchInfoMap = map[*FetchInfo]struct{}{}
)

func init() {
	const linger = time.Minute
	go func() {
		for {
			now := time.Now()
			fetchInfoMu.Lock()
			for fi := range fetchInfoMap {
				if !fi.Finish.IsZero() && now.Sub(fi.Finish) > linger {
					delete(fetchInfoMap, fi)
				}
			}
			fetchInfoMu.Unlock()
			time.Sleep(linger)
		}
	}()
}

func startFetchInfo(fi *FetchInfo) {
	fetchInfoMu.Lock()
	defer fetchInfoMu.Unlock()
	fetchInfoMap[fi] = struct{}{}
}

func finishFetchInfo(fi *FetchInfo, status int, err error) {
	fetchInfoMu.Lock()
	defer fetchInfoMu.Unlock()
	fi.Finish = time.Now()
	fi.Status = status
	fi.Error = err
}

// FetchInfos returns information about all fetches in progress,
// sorted by start time.
func FetchInfos() []*FetchInfo {
	var fis []*FetchInfo
	fetchInfoMu.Lock()
	for fi := range fetchInfoMap {
		// Copy to avoid races on Status and Error when read by
		// worker home page.
		cfi := *fi
		fis = append(fis, &cfi)
	}
	fetchInfoMu.Unlock()
	// Order first by done-ness, then by age.
	sort.Slice(fis, func(i, j int) bool {
		if (fis[i].Status == 0) == (fis[j].Status == 0) {
			return fis[i].Start.Before(fis[j].Start)
		}
		return fis[i].Status == 0
	})
	return fis
}
