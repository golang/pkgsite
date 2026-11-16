// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licensedetect

import (
	"context"
	"strings"

	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
)

// Files we should ignore.
// Set of "modulePath filePath".
var ignoreFiles = map[string]bool{
	// We told Elias Naur to put a sentence about dual-licensing into his COPYING file.
	// We no longer need it (since we now accept both licenses), but it fails detection
	// so we have to ignore it.
	"gioui.org COPYING": true,
}

//go:generate rm -f exceptions.gen.go
//go:generate go run gen_exceptions.go

func init() {
	for _, f := range licenses.FileNames {
		fileNamesLowercase[strings.ToLower(f)] = true
	}
	// exceptionTypes is a map from License IDs from LREs in the exception
	// directory to license types. Any type mentioned in an exception should
	// be redistributable. If not, there's a problem.
	for _, types := range exceptionTypes {
		for _, t := range types {
			if !licenses.Redistributable([]string{t}) {
				log.Fatalf(context.Background(), "%s is an exception type that is not redistributable.", t)
			}
		}
	}
}
