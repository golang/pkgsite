// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package godoc returns a section of the dochtml, based on the template from
// internal/fetch/dochtml.
//
// This package is intended to aid frontend development for the unit page,
// and is a temporary solution for prototyping the new designs. It is not
// intended for use in production.
package godoc

import (
	"fmt"
	"html"
	"regexp"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch/dochtml"
)

// SectionType is a section of the docHTML.
type SectionType uint32

const (
	// SidenavSection is the section of the docHTML starting at
	// dochtml.IdentifierSidenav and ending at the last </nav> tag in the
	// docHTML.
	SidenavSection SectionType = iota
	// BodySection is the section of the docHTML starting at
	// dochtml.IdentifierBody and ending at the last </div> tag in the
	// docHTML.
	BodySection
)

// Parse return the section of docHTML specified by section. It is expected that
// docHTML was generated using the template in internal/fetch/dochtml.
func Parse(docHTML safehtml.HTML, section SectionType) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "Parse(docHTML, %q)", section)
	switch section {
	case SidenavSection:
		return findHTML(docHTML, dochtml.IdentifierSidenavStart)
	case BodySection:
		return findHTML(docHTML, dochtml.IdentifierBodyStart)
	default:
		return safehtml.HTML{}, derrors.NotFound
	}
}

func findHTML(docHTML safehtml.HTML, identifier string) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "findHTML(%q)", identifier)
	var closeTag string
	switch identifier {
	case dochtml.IdentifierBodyStart:
		closeTag = dochtml.IdentifierBodyEnd
	default:
		closeTag = dochtml.IdentifierSidenavEnd
	}

	// The regex is greedy, so it will capture the last matching closeTag in
	// the docHTML.
	//
	// For the sidenav, this will capture both the mobile and main
	// sidenav sections. For the body, this will capture all content up the
	// last </div> tag in the HTML.
	reg := fmt.Sprintf("(%s(.|\n)*%s)", identifier, closeTag)
	b := regexp.MustCompile(reg).FindString(html.UnescapeString(docHTML.String()))
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(string(b)), nil
}
