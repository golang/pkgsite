// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DocumentationDetails contains data for the doc template.
type DocumentationDetails struct {
	GOOS          string
	GOARCH        string
	Documentation safehtml.HTML
}

// fetchDocumentationDetails returns a DocumentationDetails constructed from doc.
func fetchDocumentationDetails(doc *internal.Documentation) *DocumentationDetails {
	return &DocumentationDetails{
		GOOS:          doc.GOOS,
		GOARCH:        doc.GOARCH,
		Documentation: doc.HTML,
	}
}

// fileSource returns the original filepath in the module zip where the given
// filePath can be found. For std, the corresponding URL in
// go.google.source.com/go is returned.
func fileSource(modulePath, version, filePath string) string {
	if modulePath != stdlib.ModulePath {
		return fmt.Sprintf("%s@%s/%s", modulePath, version, filePath)
	}

	root := strings.TrimPrefix(stdlib.GoRepoURL, "https://")
	tag, err := stdlib.TagForVersion(version)
	if err != nil {
		// This should never happen unless there is a bug in
		// stdlib.TagForVersion. In which case, fallback to the default
		// zipFilePath.
		log.Errorf(context.TODO(), "fileSource: %v", err)
		return fmt.Sprintf("%s/+/refs/heads/master/%s", root, filePath)
	}
	return fmt.Sprintf("%s/+/refs/tags/%s/%s", root, tag, filePath)
}
