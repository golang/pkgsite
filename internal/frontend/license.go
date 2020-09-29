// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"sort"
	"strconv"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
)

// License contains information used for a single license section.
type License struct {
	*licenses.License
	Anchor safehtml.Identifier
	Source string
}

// LicensesDetails contains license information for a package or module.
type LicensesDetails struct {
	Licenses []License
}

// LicenseMetadata contains license metadata that is used in the package
// header.
type LicenseMetadata struct {
	Type   string
	Anchor safehtml.Identifier
}

// fetchLicensesDetails fetches license data for the package version specified by
// path and version from the database and returns a LicensesDetails.
func fetchLicensesDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) (*LicensesDetails, error) {
	u, err := ds.GetUnit(ctx, um, internal.WithLicenses)
	if err != nil {
		return nil, err
	}
	return &LicensesDetails{Licenses: transformLicenses(um.ModulePath, um.Version, u.LicenseContents)}, nil
}

// transformLicenses transforms licenses.License into a License
// by adding an anchor field.
func transformLicenses(modulePath, requestedVersion string, dbLicenses []*licenses.License) []License {
	licenses := make([]License, len(dbLicenses))
	var filePaths []string
	for _, l := range dbLicenses {
		filePaths = append(filePaths, l.FilePath)
	}
	anchors := licenseAnchors(filePaths)
	for i, l := range dbLicenses {
		l.Contents = bytes.ReplaceAll(l.Contents, []byte("\r"), nil)
		licenses[i] = License{
			Anchor:  anchors[i],
			License: l,
			Source:  fileSource(modulePath, requestedVersion, l.FilePath),
		}
	}
	return licenses
}

// transformLicenseMetadata transforms licenses.Metadata into a LicenseMetadata
// by adding an anchor field.
func transformLicenseMetadata(dbLicenses []*licenses.Metadata) []LicenseMetadata {
	var mds []LicenseMetadata
	var filePaths []string
	for _, l := range dbLicenses {
		filePaths = append(filePaths, l.FilePath)
	}
	anchors := licenseAnchors(filePaths)
	for i, l := range dbLicenses {
		anchor := anchors[i]
		for _, typ := range l.Types {
			mds = append(mds, LicenseMetadata{
				Type:   typ,
				Anchor: anchor,
			})
		}
	}
	return mds
}

// licenseAnchors returns anchors (HTML identifiers) for all the paths, in the
// same order. If the paths are unique, it ensures that the resulting anchors
// are unique. The argument is modified.
func licenseAnchors(paths []string) []safehtml.Identifier {
	// Remember the original index of each path.
	index := map[string]int{}
	for i, p := range paths {
		index[p] = i
	}
	// Pick a canonical order for the paths, so we assign the same anchors
	// the same set of paths regardless of the order they're given to use.
	sort.Strings(paths)
	ids := make([]safehtml.Identifier, len(paths))
	for i, p := range paths {
		ids[index[p]] = safehtml.IdentifierFromConstantPrefix("lic", strconv.Itoa(i))
	}
	return ids
}
