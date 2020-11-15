// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// Package contains information for an individual package.
type Package struct {
	Module
	Path               string // full import path
	URL                string // relative to this site
	LatestURL          string // link with latest-version placeholder, relative to this site
	IsRedistributable  bool
	Licenses           []LicenseMetadata
	PathAfterDirectory string // for display on the directories tab; used by Directory
	Synopsis           string // for display on the directories tab; used by Directory
}

// Module contains information for an individual module.
type Module struct {
	DisplayVersion    string
	LinkVersion       string
	ModulePath        string
	CommitTime        string
	IsRedistributable bool
	URL               string // relative to this site
	LatestURL         string // link with latest-version placeholder, relative to this site
	Licenses          []LicenseMetadata
}

func constructPackageURL(pkgPath, modulePath, linkVersion string) string {
	if linkVersion == internal.LatestVersion {
		return "/" + pkgPath
	}
	if pkgPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", pkgPath, linkVersion)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, linkVersion, strings.TrimPrefix(pkgPath, modulePath+"/"))
}

// absoluteTime takes a date and returns returns a human-readable,
// date with the format mmm d, yyyy:
func absoluteTime(date time.Time) string {
	return date.Format("Jan _2, 2006")
}
