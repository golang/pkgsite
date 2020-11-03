// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The functions in this file are intended to aid frontend development for the
// unit page, and is a temporary solution for prototyping the new designs. It is
// not intended for use in production.

package godoc

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
)

// SectionType is a section of the docHTML.
type SectionType uint32

const (
	// SidenavSection is the section of the docHTML starting at
	// dochtml.IdentifierSidenav and ending at the last </nav> tag in the
	// docHTML. This is inclusive of SideNavMobile.
	SidenavSection SectionType = iota
	// SidenavMobileSection is the section of the docHTML starting at
	// dochtml.IdentifierMobileNavStart and ending at the last </nav> tag in the
	// docHTML.
	SidenavMobileSection
	// BodySection is the section of the docHTML starting at
	// dochtml.IdentifierBody and ending at the last </div> tag in the
	// docHTML.
	BodySection
)

const (
	IdentifierBodyStart          = `<div class="Documentation-content js-docContent">`
	IdentifierBodyEnd            = `</div>`
	IdentifierSidenavStart       = `<nav class="DocNav js-sideNav">`
	IdentifierSidenavMobileStart = `<nav class="DocNavMobile js-mobileNav">`
	IdentifierSidenavEnd         = `</nav>`
)

// ParseDoc splits docHTML into sections.
func ParseDoc(ctx context.Context, docHTML safehtml.HTML) (body, outline, mobileOutline safehtml.HTML, err error) {
	// TODO: Deprecate this. The sidenav and body can
	// either be rendered using separate functions, or all this content can
	// be passed to the template via the UnitPage struct.
	defer middleware.ElapsedStat(ctx, "godoc.ParseDoc")
	b, err := parse(docHTML, BodySection)
	if err != nil {
		return safehtml.HTML{}, safehtml.HTML{}, safehtml.HTML{}, err
	}
	o, err := parse(docHTML, SidenavSection)
	if err != nil {
		return safehtml.HTML{}, safehtml.HTML{}, safehtml.HTML{}, err
	}
	m, err := parse(docHTML, SidenavMobileSection)
	if err != nil {
		return safehtml.HTML{}, safehtml.HTML{}, safehtml.HTML{}, err
	}
	return b, o, m, nil
}

// parse returns the section of docHTML specified by section. It is expected that
// docHTML was generated using the template in internal/fetch/dochtml.
func parse(docHTML safehtml.HTML, section SectionType) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "Parse(docHTML, %q)", section)
	switch section {
	case SidenavSection:
		return findHTML(docHTML, IdentifierSidenavStart)
	case SidenavMobileSection:
		return findHTML(docHTML, IdentifierSidenavMobileStart)
	case BodySection:
		return findHTML(docHTML, IdentifierBodyStart)
	default:
		return safehtml.HTML{}, derrors.NotFound
	}
}

func findHTML(docHTML safehtml.HTML, identifier string) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "findHTML(%q)", identifier)
	var closeTag string
	switch identifier {
	case IdentifierBodyStart:
		closeTag = IdentifierBodyEnd
	default:
		closeTag = IdentifierSidenavEnd
	}

	// The regex is greedy, so it will capture the last matching closeTag in
	// the docHTML.
	//
	// For the sidenav, this will capture both the mobile and main
	// sidenav sections. For the body, this will capture all content up the
	// last </div> tag in the HTML.
	reg := fmt.Sprintf("(%s(.|\n)*%s)", identifier, closeTag)
	s := regexp.MustCompile(reg).FindString(docHTML.String())
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s), nil
}
