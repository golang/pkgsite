// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

// Limits for discovery worker.
const (
	maxPackagesPerModule = 10000
	maxImportsPerPackage = 1000

	// MaxFileSize is the maximum filesize that is allowed for reading.
	// The fetch process should fail if it encounters a file exceeding
	// this limit.
	MaxFileSize = 30 * megabyte
)

// MaxDocumentationHTML is a limit on the rendered documentation HTML size.
//
// The current limit of is based on the largest packages that
// pkg.go.dev has encountered. See https://golang.org/issue/40576.
//
// It is a variable for testing.
var MaxDocumentationHTML = 20 * megabyte

const megabyte = 1000 * 1000
