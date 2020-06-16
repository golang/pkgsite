// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
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
func constructOverviewDetails(ctx context.Context, mi *internal.ModuleInfo, readme *internal.Readme, isRedistributable bool, versionedLinks bool) *OverviewDetails {
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
	if overview.Redistributable && readme != nil {
		overview.ReadMeSource = fileSource(mi.ModulePath, mi.Version, readme.Filepath)
		overview.ReadMe = readmeHTML(ctx, mi, readme)
	}
	return overview
}

// fetchPackageOverviewDetails uses data for the given package to return an OverviewDetails.
func fetchPackageOverviewDetails(ctx context.Context, pkg *internal.LegacyVersionedPackage, versionedLinks bool) *OverviewDetails {
	od := constructOverviewDetails(ctx, &pkg.ModuleInfo, &internal.Readme{Filepath: pkg.LegacyReadmeFilePath, Contents: pkg.LegacyReadmeContents},
		pkg.LegacyPackage.IsRedistributable, versionedLinks)
	od.PackageSourceURL = pkg.SourceInfo.DirectoryURL(packageSubdir(pkg.Path, pkg.ModulePath))
	if !pkg.LegacyPackage.IsRedistributable {
		od.Redistributable = false
	}
	return od
}

// fetchPackageOverviewDetailsNew uses data for the given versioned directory to return an OverviewDetails.
func fetchPackageOverviewDetailsNew(ctx context.Context, vdir *internal.VersionedDirectory, versionedLinks bool) *OverviewDetails {
	var lv string
	if versionedLinks {
		lv = linkVersion(vdir.Version, vdir.ModulePath)
	} else {
		lv = internal.LatestVersion
	}
	overview := &OverviewDetails{
		ModulePath:       vdir.ModulePath,
		ModuleURL:        constructModuleURL(vdir.ModulePath, lv),
		RepositoryURL:    vdir.SourceInfo.RepoURL(),
		Redistributable:  vdir.DirectoryNew.IsRedistributable,
		PackageSourceURL: vdir.SourceInfo.DirectoryURL(packageSubdir(vdir.Path, vdir.ModulePath)),
	}
	if overview.Redistributable && vdir.Readme != nil {
		overview.ReadMeSource = fileSource(vdir.ModulePath, vdir.Version, vdir.Readme.Filepath)
		overview.ReadMe = readmeHTML(ctx, &vdir.ModuleInfo, vdir.Readme)
	}
	return overview
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
func readmeHTML(ctx context.Context, mi *internal.ModuleInfo, readme *internal.Readme) template.HTML {
	if readme == nil {
		return ""
	}
	if !isMarkdown(readme.Filepath) {
		return template.HTML(fmt.Sprintf(`<pre class="readme">%s</pre>`, html.EscapeString(readme.Contents)))
	}

	// bluemonday.UGCPolicy allows a broad selection of HTML elements and
	// attributes that are safe for user generated content. This policy does
	// not allow iframes, object, embed, styles, script, etc.
	p := bluemonday.UGCPolicy()

	// Allow width and align attributes on img, div, and p tags.
	// This is used to center elements in a readme as well as to size it
	// images appropriately where used, like the gin-gonic/logo/color.png
	// image in the github.com/gin-gonic/gin README.
	p.AllowAttrs("width", "align").OnElements("img")
	p.AllowAttrs("width", "align").OnElements("div")
	p.AllowAttrs("width", "align").OnElements("p")

	// blackfriday.Run() uses CommonHTMLFlags and CommonExtensions by default.
	renderer := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{Flags: blackfriday.CommonHTMLFlags})
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions | blackfriday.AutoHeadingIDs))

	// Render HTML similar to blackfriday.Run(), but here we implement a custom
	// Walk function in order to modify image paths in the rendered HTML.
	b := &bytes.Buffer{}
	rootNode := parser.Parse([]byte(readme.Contents))
	rootNode.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		switch node.Type {
		case blackfriday.Image, blackfriday.Link:
			useRaw := node.Type == blackfriday.Image
			if d := translateRelativeLink(string(node.LinkData.Destination), mi, useRaw, readme); d != "" {
				node.LinkData.Destination = []byte(d)
			}
		case blackfriday.HTMLBlock, blackfriday.HTMLSpan:
			if experiment.IsActive(ctx, internal.ExperimentTranslateHTML) {
				d, err := translateHTML(node.Literal, mi, readme)
				if err != nil {
					log.Errorf(context.Background(), "couldn't transform html block(%s): %v", node.Literal, err)
				} else {
					node.Literal = d
				}
			}
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

// translateRelativeLink converts relative image paths to absolute paths.
//
// README files sometimes use relative image paths to image files inside the
// repository. As the discovery site doesn't host the full repository content,
// in order for the image to render, we need to convert the relative path to an
// absolute URL to a hosted image.
func translateRelativeLink(dest string, mi *internal.ModuleInfo, useRaw bool, readme *internal.Readme) string {
	destURL, err := url.Parse(dest)
	if err != nil || destURL.IsAbs() {
		return ""
	}
	if destURL.Path == "" {
		// This is a fragment; leave it.
		return ""
	}
	// Paths are relative to the README location.
	destPath := path.Join(path.Dir(readme.Filepath), path.Clean(destURL.Path))
	if useRaw {
		return mi.SourceInfo.RawURL(destPath)
	}
	return mi.SourceInfo.FileURL(destPath)
}

// translateHTML parses html text into parsed html nodes. It then
// iterates through the nodes and replaces the src key with a value
// that properly represents the source of the image from the repo.
func translateHTML(htmlText []byte, mi *internal.ModuleInfo, readme *internal.Readme) ([]byte, error) {
	r := bytes.NewReader(htmlText)
	nodes, err := html.ParseFragment(r, nil)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		// Every parsed node begins with <html><head></head><body>. Ignore that.
		if n.DataAtom != atom.Html {
			return htmlText, nil
		}
		// When the parsed html nodes don't have a valid structure
		// (i.e: an html comment), then just return the original text.
		if n.FirstChild == nil || n.FirstChild.NextSibling == nil || n.FirstChild.NextSibling.DataAtom != atom.Body {
			return htmlText, nil
		}
		n = n.FirstChild.NextSibling.FirstChild
		// If <html><head><body> </body>... has no children (empty content),
		// then just return the original text.
		if n == nil {
			return htmlText, nil
		}
		walkHTML(n, mi, readme)
		if err := html.Render(&buf, n); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// walkHTML crawls through an html node and replaces the src
// tag link with a link that properly represents the image
// from the repo source.
func walkHTML(n *html.Node, mi *internal.ModuleInfo, readme *internal.Readme) {
	if n.Type == html.ElementNode && n.DataAtom == atom.Img {
		var attrs []html.Attribute
		for _, a := range n.Attr {
			if a.Key == "src" {
				if v := translateRelativeLink(a.Val, mi, true, readme); v != "" {
					a.Val = v
				}
			}
			attrs = append(attrs, a)
		}
		n.Attr = attrs
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, mi, readme)
	}
}
