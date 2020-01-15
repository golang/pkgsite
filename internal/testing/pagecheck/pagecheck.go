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

	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/htmlcheck"
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
	in        = htmlcheck.In
	inAll     = htmlcheck.InAll
	text      = htmlcheck.HasText
	exactText = htmlcheck.HasExactText
	attr      = htmlcheck.HasAttr
	href      = htmlcheck.HasHref
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
		in("h1.DetailsHeader-title", exactText(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		versionBadge(p),
		licenseInfo(p, packageURLPath(p, versionedURL), versionedURL),
		packageTabLinks(p, versionedURL),
		moduleInHeader(p, versionedURL))
}

// ModuleHeader checks a details page header for a module.
func ModuleHeader(p *Page, versionedURL bool) htmlcheck.Checker {
	fv := p.FormattedVersion
	if fv == "" {
		fv = p.Version
	}
	return in("",
		in("h1.DetailsHeader-title", exactText(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		versionBadge(p),
		licenseInfo(p, moduleURLPath(p, versionedURL), versionedURL),
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
		in("h1.DetailsHeader-title", exactText(p.Title)),
		in("div.DetailsHeader-version", exactText(fv)),
		// directory pages don't show a header badge
		in("div.DetailsHeader-badge", in(".DetailsHeader-unknown")),
		licenseInfo(p, packageURLPath(p, versionedURL), versionedURL),
		// directory module links are always versioned (see b/144217401)
		moduleInHeader(p, true))
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
	class := "DetailsHeader-"
	if p.IsLatest {
		class += "latest"
	} else {
		class += "goToLatest"
	}
	return in("div.DetailsHeader-badge",
		attr("class", `\b`+regexp.QuoteMeta(class)+`\b`), // the badge has this class too
		in("a", href(p.LatestLink), exactText("Go to latest")))
}

// licenseInfo checks the license part of the info label in the header.
func licenseInfo(p *Page, urlPath string, versionedURL bool) htmlcheck.Checker {
	if p.LicenseType == "" {
		return in("[data-test-id=DetailsHeader-infoLabelLicense]", text("None detected"))
	}
	return in("[data-test-id=DetailsHeader-infoLabelLicense] a",
		href(fmt.Sprintf("%s?tab=licenses#%s", urlPath, p.LicenseFilePath)),
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
