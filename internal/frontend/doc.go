// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DocumentationDetails contains data for the doc template.
type DocumentationDetails struct {
	GOOS          string
	GOARCH        string
	Documentation safehtml.HTML
}

// fetchDocumentationDetails returns a DocumentationDetails.
func fetchDocumentationDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) (_ *DocumentationDetails, err error) {
	derrors.Wrap(&err, "fetchDocumentationDetails(%q, %q, %q)", um.Path, um.ModulePath, um.Version)

	u, err := ds.GetUnit(ctx, um, internal.WithDocumentation)
	if err != nil {
		return nil, err
	}
	if experiment.IsActive(ctx, internal.ExperimentFrontendRenderDoc) && len(u.Documentation.Source) > 0 {
		docPkg, err := godoc.DecodePackage(u.Documentation.Source)
		if err != nil {
			return nil, err
		}
		dd, err := renderDoc(ctx, u, docPkg)
		if err != nil {
			log.Errorf(ctx, "render doc failed: %v", err)
			// Fall through to use stored doc.
		} else {
			return dd, nil
		}
	}
	return &DocumentationDetails{
		GOOS:          u.Documentation.GOOS,
		GOARCH:        u.Documentation.GOARCH,
		Documentation: u.Documentation.HTML,
	}, nil
}

func renderDoc(ctx context.Context, u *internal.Unit, docPkg *godoc.Package) (_ *DocumentationDetails, err error) {
	defer derrors.Wrap(&err, "renderDoc")
	defer middleware.ElapsedStat(ctx, "renderDoc")()

	start := time.Now()
	modInfo := &godoc.ModuleInfo{
		ModulePath:      u.ModulePath,
		ResolvedVersion: u.Version,
		ModulePackages:  nil, // will be provided by docPkg
	}
	var innerPath string
	if u.ModulePath == stdlib.ModulePath {
		innerPath = u.Path
	} else if u.Path != u.ModulePath {
		innerPath = u.Path[len(u.ModulePath)+1:]
	}
	_, _, html, err := docPkg.Render(ctx, innerPath, u.SourceInfo, modInfo, "", "")
	if err != nil {
		return nil, err
	}
	log.Infof(ctx, "rendered doc for %s@%s in %s", u.Path, u.Version, time.Since(start))
	return &DocumentationDetails{
		GOOS:          docPkg.GOOS,
		GOARCH:        docPkg.GOARCH,
		Documentation: html,
	}, nil
}

func renderDocParts(ctx context.Context, u *internal.Unit, docPkg *godoc.Package) (body, outline, mobileOutline safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "renderDocParts")
	defer middleware.ElapsedStat(ctx, "renderDocParts")()

	modInfo := &godoc.ModuleInfo{
		ModulePath:      u.ModulePath,
		ResolvedVersion: u.Version,
		ModulePackages:  nil, // will be provided by docPkg
	}
	var innerPath string
	if u.ModulePath == stdlib.ModulePath {
		innerPath = u.Path
	} else if u.Path != u.ModulePath {
		innerPath = u.Path[len(u.ModulePath)+1:]
	}
	return docPkg.RenderParts(ctx, innerPath, u.SourceInfo, modInfo)
}

// sourceFiles returns the .go files for a package.
func sourceFiles(u *internal.Unit, docPkg *godoc.Package) []*File {
	var files []*File
	for _, f := range docPkg.Files {
		if strings.HasSuffix(f.Name, "_test.go") {
			continue
		}
		files = append(files, &File{
			Name: f.Name,
			URL:  u.SourceInfo.FileURL(path.Join(internal.Suffix(u.Path, u.ModulePath), f.Name)),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
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
