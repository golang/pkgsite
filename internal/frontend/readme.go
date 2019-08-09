// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

// ReadMeDetails contains all of the data that the readme template
// needs to populate.
type ReadMeDetails struct {
	ModulePath string
	ReadMe     template.HTML
}

// fetchReadMeDetails fetches data for the module version specified by path and version
// from the database and returns a ReadMeDetails.
func fetchReadMeDetails(ctx context.Context, db *postgres.DB, vi *internal.VersionInfo) (*ReadMeDetails, error) {
	return &ReadMeDetails{
		ModulePath: vi.ModulePath,
		ReadMe:     readmeHTML(vi),
	}, nil
}

// readmeHTML sanitizes readmeContents based on bluemondy.UGCPolicy and returns
// a template.HTML. If readmeFilePath indicates that this is a markdown file,
// it will also render the markdown contents using blackfriday.
func readmeHTML(vi *internal.VersionInfo) template.HTML {
	if filepath.Ext(vi.ReadmeFilePath) != ".md" {
		return template.HTML(fmt.Sprintf(`<pre class="readme">%s</pre>`, html.EscapeString(string(vi.ReadmeContents))))
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
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions))

	// Render HTML similar to blackfriday.Run(), but here we implement a custom
	// Walk function in order to modify image paths in the rendered HTML.
	b := &bytes.Buffer{}
	rootNode := parser.Parse(vi.ReadmeContents)
	rootNode.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if node.Type == blackfriday.Image {
			translateRelativeLink(node, vi)
		}
		return renderer.RenderNode(b, node, entering)
	})
	return template.HTML(p.SanitizeReader(b).String())
}

// translateRelativeLink modifies a blackfriday.Node to convert relative image
// paths to absolute paths.
//
// Markdown files, such as the Go README, sometimes use relative image paths to
// image files inside the repository. As the discovery site doesn't host the
// full repository content, in order for the image to render, we need to
// convert the relative path to an absolute URL to a hosted image.
func translateRelativeLink(node *blackfriday.Node, vi *internal.VersionInfo) {
	repo, err := url.Parse(vi.RepositoryURL)
	if err != nil || repo.Hostname() != "github.com" {
		return
	}
	imageURL, err := url.Parse(string(node.LinkData.Destination))
	if err != nil || imageURL.IsAbs() {
		return
	}
	ref := "master"
	switch vi.VersionType {
	case internal.VersionTypeRelease, internal.VersionTypePrerelease:
		ref = vi.Version
		if internal.IsStandardLibraryModule(vi.ModulePath) {
			ref, err = internal.GoVersionForSemanticVersion(ref)
			if err != nil {
				ref = "master"
			}
		}
	case internal.VersionTypePseudo:
		if segs := strings.SplitAfter(vi.Version, "-"); len(segs) != 0 {
			ref = segs[len(segs)-1]
		}
	}
	abs := &url.URL{Scheme: "https", Host: "raw.githubusercontent.com", Path: path.Join(repo.Path, ref, path.Clean(imageURL.Path))}
	node.LinkData.Destination = []byte(abs.String())
}
