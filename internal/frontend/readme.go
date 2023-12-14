// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"golang.org/x/pkgsite/internal/sanitizer"
)

// Heading holds data about a heading and nested headings within a readme.
// This data is used in the sidebar template to render the readme outline.
type Heading struct {
	// Level is the original level of the heading.
	Level int
	// Text is the content from the readme contained within a heading.
	Text string
	// ID corresponds to the ID attribute for a heading element
	// and is also used in an href to the corresponding section
	// within the readme outline. All ids are prefixed with readme-
	// to avoid name collisions.
	ID string
	// Children are nested headings.
	Children []*Heading
	// parent is the heading this heading is nested within. Nil for top
	// level headings.
	parent *Heading
}

// Readme holds the result of processing a REAME file.
type Readme struct {
	HTML    safehtml.HTML // rendered HTML
	Outline []*Heading    // document headings
	Links   []link        // links from the "Links" section
}

// sanitizeHTML sanitizes HTML from a bytes.Buffer so that it is safe.
func sanitizeHTML(b *bytes.Buffer) safehtml.HTML {
	s := string(sanitizer.SanitizeBytes(b.Bytes()))
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s)
}
