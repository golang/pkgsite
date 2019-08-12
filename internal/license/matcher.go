// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"path"
	"path/filepath"
	"strings"
)

// Matcher associates directories with their applicable licenses.
type Matcher map[string][]*Metadata

// NewMatcher creates a Matcher for the given licenses.
func NewMatcher(licenses []*License) Matcher {
	var matcher Matcher = make(map[string][]*Metadata)
	for _, l := range licenses {
		prefix := path.Dir(l.FilePath)
		matcher[prefix] = append(matcher[prefix], l.Metadata)
	}
	return matcher
}

// Match returns the slice of licenses that apply to the given directory.  A
// license applies to a directory if it is contained in that directory or any
// parent directory up to and including the root.
func (m Matcher) Match(dir string) []*Metadata {
	cleanDir := filepath.ToSlash(filepath.Clean(dir))
	if path.IsAbs(cleanDir) || strings.HasPrefix(cleanDir, "..") {
		return nil
	}

	var licenseFiles []*Metadata
	for prefix, lms := range m {
		// append a slash so that prefix a/b does not match a/bc/d
		if prefix == "." || strings.HasPrefix(cleanDir+"/", prefix+"/") {
			licenseFiles = append(licenseFiles, lms...)
		}
	}
	return licenseFiles
}
