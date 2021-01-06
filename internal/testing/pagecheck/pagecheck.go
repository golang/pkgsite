// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pagecheck implements HTML checkers for discovery site pages.
// It uses the general-purpose checkers in internal/testing/htmlcheck to define
// site-specific checkers.
package pagecheck

import (
	"fmt"
	"path"
	"regexp"

	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

// Page describes a discovery site web page for a package, module or directory.
type Page struct {
	// ModulePath is the module path for the unit page.
	ModulePath string

	// Suffix is the unit path element after module path; empty for a module
	Suffix string

	// Version is the full version of the module, or the go tag if it is the
	// stdlib.
	Version string

	// FormattedVersion is the version of the module, or go tag if it is the
	// stdlib. The version string may be truncated if it is a pseudoversion.
	FormattedVersion string

	// Title is output of frontend.pageTitle.
	Title string

	// LicenseType is name of the license.
	LicenseType string

	// LicenseFilePath is the path of the license relative to the module directory.
	LicenseFilePath string

	// IsLatestMinor is the latest minor version of this module.
	IsLatestMinor bool

	// MissingInMinor says that the unit is missing in the latest minor version of this module.
	MissingInMinor bool

	// IsLatestMajor is the latest major version of this series.
	IsLatestMajor bool

	// LatestLink is the href of "Go to latest" link.
	LatestLink string

	// LatestMajorVersion is the suffix of the latest major version, empty if
	// v0 or v1.
	LatestMajorVersion string

	// LatestMajorVersionLink is the link to the latest major version of the
	// unit. If the unit does not exist at the latest major version, it is a
	// link to the latest major version of the module.
	LatestMajorVersionLink string

	// UnitURLFormat is the relative unit URL, with one %s for "@version".
	UnitURLFormat string

	// ModuleURL is the relative module URL.
	ModuleURL string

	// CommitTime is the output of frontend.absoluteTime for the commit time.
	CommitTime string
}

var (
	in        = htmlcheck.In
	text      = htmlcheck.HasText
	exactText = htmlcheck.HasExactText
	attr      = htmlcheck.HasAttr
	href      = htmlcheck.HasHref
)

// UnitHeader checks a main page header for a unit.
func UnitHeader(p *Page, versionedURL bool, isPackage bool) htmlcheck.Checker {
	urlPath := unitURLPath(p, versionedURL)
	curBreadcrumb := path.Base(p.Suffix)
	if p.Suffix == "" {
		curBreadcrumb = p.ModulePath
	}
	licenseText := p.LicenseType
	licenseLink := urlPath + "?tab=licenses"
	if p.LicenseType == "" {
		licenseText = "not legal advice"
		licenseLink = "/license-policy"
	}

	importsDetails := in("",
		in(`[data-test-id="UnitHeader-imports"]`,
			in("a",
				href(urlPath+"?tab=imports"),
				text(`[0-9]+\+? Imports`))),
		in(`[data-test-id="UnitHeader-importedby"]`,
			in("a",
				href(urlPath+"?tab=importedby"),
				text(`[0-9]+\+? Imported by`))))
	if !isPackage {
		importsDetails = nil
	}

	majorVersionBannerClass := "UnitHeader-majorVersionBanner"
	if p.IsLatestMajor {
		majorVersionBannerClass += "  DetailsHeader-banner--latest"
	}

	return in("header.UnitHeader",
		versionBadge(p),
		in(`[data-test-id="UnitHeader-breadcrumbCurrent"]`, text(curBreadcrumb)),
		in(`[data-test-id="UnitHeader-title"]`, text(p.Title)),
		in(`[data-test-id="UnitHeader-majorVersionBanner"]`,
			attr("class", majorVersionBannerClass),
			in("span",
				text("The highest tagged major version is "),
				in("a",
					href(p.LatestMajorVersionLink),
					exactText(p.LatestMajorVersion),
				),
			),
		),
		in(`[data-test-id="UnitHeader-version"]`,
			in("a",
				href("?tab=versions"),
				exactText("Version "+p.FormattedVersion))),
		in(`[data-test-id="UnitHeader-commitTime"]`,
			text(p.CommitTime)),
		in(`[data-test-id="UnitHeader-licenses"]`,
			in("a",
				href(licenseLink),
				text(licenseText))),
		importsDetails)
}

// UnitReadme checks the readme section of the main page.
func UnitReadme() htmlcheck.Checker {
	return in(".UnitReadme",
		in(`[data-test-id="Unit-readmeContent"]`, text("readme")),
	)
}

// UnitDoc checks the doc section of the main page.
func UnitDoc() htmlcheck.Checker {
	return in(".Documentation", text(`Overview`))
}

// UnitDirectories checks the directories section of the main page.
// If firstHref isn't empty, it and firstText should exactly match
// href and text of the first link in the Directories table.
func UnitDirectories(firstHref, firstText string) htmlcheck.Checker {
	var link htmlcheck.Checker
	if firstHref != "" {
		link = in(`[data-test-id="UnitDirectories-table"] a`, href(firstHref), exactText(firstText))
	}
	return in("",
		in("th:nth-child(1)", text("^Path$")),
		in("th:nth-child(2)", text("^Synopsis$")),
		link)
}

// CanonicalURLPath checks the canonical url for the unit on the page.
func CanonicalURLPath(path string) htmlcheck.Checker {
	return in(".js-canonicalURLPath", attr("data-canonical-url-path", path))
}

// SubdirectoriesDetails checks the detail section of a subdirectories tab.
// If firstHref isn't empty, it and firstText should exactly match
// href and text of the first link in the Directories table.
func SubdirectoriesDetails(firstHref, firstText string) htmlcheck.Checker {
	var link htmlcheck.Checker
	if firstHref != "" {
		link = in("table.Directories a", href(firstHref), exactText(firstText))
	}
	return in("",
		in("th:nth-child(1)", text("^Path$")),
		in("th:nth-child(2)", text("^Synopsis$")),
		link)
}

// LicenseDetails checks the details section of a license tab.
func LicenseDetails(ltype, bodySubstring, source string) htmlcheck.Checker {
	return in("",
		in(".License",
			text(regexp.QuoteMeta(ltype)),
			text("This is not legal advice"),
			in("a",
				href("/license-policy"),
				exactText("Read disclaimer.")),
			in(".License-contents",
				text(regexp.QuoteMeta(bodySubstring)))),
		in(".License-source",
			exactText("Source: "+source)))
}

// versionBadge checks the latest-version badge on a header.
func versionBadge(p *Page) htmlcheck.Checker {
	class := "DetailsHeader-badge"
	switch {
	case p.MissingInMinor:
		class += "--notAtLatest"
	case p.IsLatestMinor:
		class += "--latest"
	default:
		class += "--goToLatest"
	}
	return in(`[data-test-id="UnitHeader-minorVersionBanner"]`,
		attr("class", `\b`+regexp.QuoteMeta(class)+`\b`), // the badge has this class too
		in("a", href(p.LatestLink), exactText("Go to latest")))
}

func unitURLPath(p *Page, versioned bool) string {
	v := ""
	if versioned {
		v = "@" + p.Version
	}
	return fmt.Sprintf(p.UnitURLFormat, v)
}
