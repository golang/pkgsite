// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

// Limits for discovery ETL.
const (
	maxPackagesPerModule = 10000
	maxImportsPerPackage = 1000

	// maxFileSize is the maximum filesize that is allowed for reading.
	// The fetch process should fail if it encounters a file exceeding
	// this limit.
	maxFileSize = 30 * megabyte

	// maxDocumentationHTML is a limit on the rendered documentation
	// HTML size.
	//
	// The current limit of 10 MB was based on the largest packages that
	// gddo has encountered. See https://github.com/golang/gddo/issues/635.
	maxDocumentationHTML = 10 * megabyte
)

const megabyte = 1000 * 1000
