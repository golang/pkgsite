// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

// Files we should ignore.
// Set of "modulePath filePath".
var ignoreFiles = map[string]bool{
	// We told Elias Naur to put a sentence about dual-licensing into his COPYING file.
	// We no longer need it (since we now accept both licenses), but it fails detection
	// so we have to ignore it.
	"gioui.org COPYING": true,
}

// exceptionFileTypes returns the license types of the file with contents if it
// is in the list of exceptions. Otherwise it returns nil.
func exceptionFileTypes(contents []byte) []string {
	return exceptionFileTypesMap[exceptionKey(contents)]
}
