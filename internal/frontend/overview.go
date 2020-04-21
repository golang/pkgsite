// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// OverviewDetails contains all of the data that the readme template
// needs to populate.
type OverviewDetails struct {
	ModulePath       string
	ModuleURL        string
	PackageSourceURL string
	ReadMe           template.HTML
	ReadMeSource     string
	Redistributable  bool
	RepositoryURL    string
}

// versionedLinks says whether the constructed URLs should have versions.
// constructOverviewDetails uses the given version to construct an OverviewDetails.
func constructOverviewDetails(mi *internal.ModuleInfo, isRedistributable bool, versionedLinks bool) *OverviewDetails {
	var lv string
	if versionedLinks {
		lv = linkVersion(mi.Version, mi.ModulePath)
	} else {
		lv = internal.LatestVersion
	}
	overview := &OverviewDetails{
		ModulePath:      mi.ModulePath,
		ModuleURL:       constructModuleURL(mi.ModulePath, lv),
		RepositoryURL:   mi.SourceInfo.RepoURL(),
		Redistributable: isRedistributable,
	}
	if overview.Redistributable {
		overview.ReadMeSource = fileSource(mi.ModulePath, mi.Version, mi.ReadmeFilePath)
		overview.ReadMe = readmeHTML(mi)
	}
	return overview
}

// constructPackageOverviewDetails uses data for the given package to return an OverviewDetails.
func constructPackageOverviewDetails(pkg *internal.VersionedPackage, versionedLinks bool) *OverviewDetails {
	od := constructOverviewDetails(&pkg.ModuleInfo, pkg.Package.IsRedistributable, versionedLinks)
	od.PackageSourceURL = pkg.SourceInfo.DirectoryURL(packageSubdir(pkg.Path, pkg.ModulePath))
	if !pkg.Package.IsRedistributable {
		od.Redistributable = false
	}
	return od
}

// packageSubdir returns the subdirectory of the package relative to its module.
func packageSubdir(pkgPath, modulePath string) string {
	switch {
	case pkgPath == modulePath:
		return ""
	case modulePath == stdlib.ModulePath:
		return pkgPath
	default:
		return strings.TrimPrefix(pkgPath, modulePath+"/")
	}
}

// readmeHTML sanitizes readmeContents based on bluemondy.UGCPolicy and returns
// a template.HTML. If readmeFilePath indicates that this is a markdown file,
// it will also render the markdown contents using blackfriday.
func readmeHTML(mi *internal.ModuleInfo) template.HTML {
	if len(mi.ReadmeContents) == 0 {
		return ""
	}
	if !isMarkdown(mi.ReadmeFilePath) {
		return template.HTML(fmt.Sprintf(`<pre class="readme">%s</pre>`, html.EscapeString(string(mi.ReadmeContents))))
	}

	// bluemonday.UGCPolicy allows a broad selection of HTML elements and
	// attributes that are safe for user generated content. This policy does
	// not whitelist iframes, object, embed, styles, script, etc.
	p := bluemonday.UGCPolicy()

	// Allow width and align attributes on img. This is used to size README
	// images appropriately where used, like the gin-gonic/logo/color.png
	// image in the github.com/gin-gonic/gin README.
	p.AllowAttrs("width", "align").OnElements("img")

	// blackfriday.Run() uses CommonHTMLFlags and CommonExtensions by default.
	renderer := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{Flags: blackfriday.CommonHTMLFlags})
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions | blackfriday.AutoHeadingIDs))

	// Render HTML similar to blackfriday.Run(), but here we implement a custom
	// Walk function in order to modify image paths in the rendered HTML.
	b := &bytes.Buffer{}
	rootNode := parser.Parse([]byte(mi.ReadmeContents))
	rootNode.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if node.Type == blackfriday.Image || node.Type == blackfriday.Link {
			translateRelativeLink(node, mi)
		}
		return renderer.RenderNode(b, node, entering)
	})
	return template.HTML(p.SanitizeReader(b).String())
}

// isMarkdown reports whether filename says that the file contains markdown.
func isMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	// https://tools.ietf.org/html/rfc7763 mentions both extensions.
	return ext == ".md" || ext == ".markdown"
}

// translateRelativeLink modifies a blackfriday.Node to convert relative image
// paths to absolute paths.
//
// Markdown files, such as the Go README, sometimes use relative image paths to
// image files inside the repository. As the discovery site doesn't host the
// full repository content, in order for the image to render, we need to
// convert the relative path to an absolute URL to a hosted image.
func translateRelativeLink(node *blackfriday.Node, mi *internal.ModuleInfo) {
	destURL, err := url.Parse(string(node.LinkData.Destination))
	if err != nil || destURL.IsAbs() {
		return
	}

	if destURL.Path == "" {
		// This is a fragment; leave it.
		return
	}
	// Paths are relative to the README location.
	destPath := path.Join(path.Dir(mi.ReadmeFilePath), path.Clean(destURL.Path))
	var newURL string
	if node.Type == blackfriday.Image {
		newURL = mi.SourceInfo.RawURL(destPath)
	} else {
		newURL = mi.SourceInfo.FileURL(destPath)
	}
	if newURL != "" {
		node.LinkData.Destination = []byte(newURL)
	}
}
