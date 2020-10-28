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

	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

// Page describes a discovery site web page for a package, module or directory.
type Page struct {
	ModulePath       string
	Suffix           string // package or directory path after module path; empty for a module
	Version          string
	FormattedVersion string
	Title            string
	LicenseType      string
	LicenseFilePath  string
	IsLatest         bool   // is this the latest version of this module?
	LatestLink       string // href of "Go to latest" link
	PackageURLFormat string // the relative package URL, with one %s for "@version"; also used for dirs
	ModuleURL        string // the relative module URL
}

// Overview describes the contents of the overview tab.
type Overview struct {
	ModuleLink     string // relative link to module page
	ModuleLinkText string
	RepoURL        string
	PackageURL     string
	ReadmeContent  string
	ReadmeSource   string
}

var (
	in                 = htmlcheck.In
	inAll              = htmlcheck.InAll
	text               = htmlcheck.HasText
	exactText          = htmlcheck.HasExactText
	exactTextCollapsed = htmlcheck.HasExactTextCollapsed
	attr               = htmlcheck.HasAttr
	href               = htmlcheck.HasHref
)

// PackageHeader checks a details page header for a package.
func PackageHeader(p *Page, versionedURL bool) htmlcheck.Checker {
	fv := p.FormattedVersion
	if fv == "" {
		fv = p.Version
	}
	curBreadcrumb := path.Base(p.Suffix)
	if p.Suffix == "" {
		curBreadcrumb = p.ModulePath
	}
	return in("",
		in("span.DetailsHeader-breadcrumbCurrent", exactText(curBreadcrumb)),
		in("h1.DetailsHeader-title", exactTextCollapsed(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		versionBadge(p),
		licenseInfo(p, packageURLPath(p, versionedURL)),
		packageTabLinks(p, versionedURL),
		moduleInHeader(p, versionedURL))
}

// ModuleHeader checks a details page header for a module.
func ModuleHeader(p *Page, versionedURL bool) htmlcheck.Checker {
	fv := p.FormattedVersion
	if fv == "" {
		fv = p.Version
	}
	curBreadcrumb := p.ModulePath
	if p.ModulePath == stdlib.ModulePath {
		curBreadcrumb = "Standard library"
	}
	return in("",
		in("span.DetailsHeader-breadcrumbCurrent", exactText(curBreadcrumb)),
		in("h1.DetailsHeader-title", exactTextCollapsed(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		versionBadge(p),
		licenseInfo(p, moduleURLPath(p, versionedURL)),
		moduleTabLinks(p, versionedURL))
}

// DirectoryHeader checks a details page header for a directory.
func DirectoryHeader(p *Page, versionedURL bool) htmlcheck.Checker {
	fv := p.FormattedVersion
	if fv == "" {
		fv = p.Version
	}
	return in("",
		in("span.DetailsHeader-breadcrumbCurrent", exactText(path.Base(p.Suffix))),
		in("h1.DetailsHeader-title", exactTextCollapsed(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		// directory pages don't show a header badge
		in("div.DetailsHeader-version", exactText(fv)),
		licenseInfo(p, packageURLPath(p, versionedURL)),
		// directory module links are always versioned (see b/144217401)
		moduleInHeader(p, true))
}

// UnitHeader checks a main page header for a unit.
func UnitHeader(p *Page, versionedURL bool, isPackage bool) htmlcheck.Checker {
	urlPath := packageURLPath(p, versionedURL)
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
				text("[0-9]+ Imports"))),
		in(`[data-test-id="UnitHeader-importedby"]`,
			in("a",
				href(urlPath+"?tab=importedby"),
				text(`[0-9]+ Imported by`))))
	if !isPackage {
		importsDetails = nil
	}

	return in("header.UnitHeader",
		in(`[data-test-id="UnitHeader-breadcrumbCurrent"]`, text(curBreadcrumb)),
		in(`[data-test-id="UnitHeader-title"]`, text(p.Title)),
		in(`[data-test-id="UnitHeader-version"]`,
			in("a",
				href("?tab=versions"),
				exactText("Version "+p.FormattedVersion))),
		in(`[data-test-id="UnitHeader-commitTime"]`,
			text("0 hours ago")),
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

// OverviewDetails checks the details section of an overview tab.
func OverviewDetails(ov *Overview) htmlcheck.Checker {
	var pkg htmlcheck.Checker
	if ov.PackageURL != "" {
		pkg = in(".Overview-sourceCodeLink a:nth-of-type(2)",
			href(ov.PackageURL),
			exactText(ov.PackageURL))
	}
	return in("",
		in("div.Overview-module > a",
			href(ov.ModuleLink),
			exactText(ov.ModuleLinkText)),
		in(".Overview-sourceCodeLink a:nth-of-type(1)",
			href(ov.RepoURL),
			exactText(ov.RepoURL)),
		pkg,
		in(".Overview-readmeContent", text(ov.ReadmeContent)),
		in(".Overview-readmeSource", exactText("Source: "+ov.ReadmeSource)))
}

// versionBadge checks the latest-version badge on a header.
func versionBadge(p *Page) htmlcheck.Checker {
	class := "DetailsHeader-badge"
	if p.IsLatest {
		class += "--latest"
	} else {
		class += "--goToLatest"
	}
	return in("div.DetailsHeader-badge",
		attr("class", `\b`+regexp.QuoteMeta(class)+`\b`), // the badge has this class too
		in("a", href(p.LatestLink), exactText("Go to latest")))
}

// licenseInfo checks the license part of the info label in the header.
func licenseInfo(p *Page, urlPath string) htmlcheck.Checker {
	if p.LicenseType == "" {
		return in("[data-test-id=DetailsHeader-infoLabelLicense]", text("None detected"))
	}
	return in("[data-test-id=DetailsHeader-infoLabelLicense] a",
		href(fmt.Sprintf("%s?tab=licenses#lic-0", urlPath)),
		exactText(p.LicenseType))
}

// moduleInHeader checks the module part of the info label in the header.
func moduleInHeader(p *Page, versionedURL bool) htmlcheck.Checker {
	modURL := moduleURLPath(p, versionedURL)
	text := p.ModulePath
	if p.ModulePath == stdlib.ModulePath {
		text = "Standard library"
	}
	return in("a[data-test-id=DetailsHeader-infoLabelModule]", href(modURL), exactText(text))
}

// Check that all the navigation tabs link to the same package at the same version.
func packageTabLinks(p *Page, versionedURL bool) htmlcheck.Checker {
	return inAll("a.DetailsNav-link[href]",
		attr("href", "^"+regexp.QuoteMeta(packageURLPath(p, versionedURL))))
}

// Check that all the navigation tabs link to the same module at the same version.
func moduleTabLinks(p *Page, versionedURL bool) htmlcheck.Checker {
	return inAll("a.DetailsNav-link[href]",
		attr("href", "^"+regexp.QuoteMeta(moduleURLPath(p, versionedURL))))
}

func packageURLPath(p *Page, versioned bool) string {
	v := ""
	if versioned {
		v = "@" + p.Version
	}
	return fmt.Sprintf(p.PackageURLFormat, v)
}

func moduleURLPath(p *Page, versioned bool) string {
	if versioned {
		return p.ModuleURL + "@" + p.Version
	}
	return p.ModuleURL
}
