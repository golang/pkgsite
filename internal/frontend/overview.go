// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/source"
)

// OverviewDetails contains all of the data that the readme template
// needs to populate.
type OverviewDetails struct {
	ModulePath       string
	ModuleURL        string
	PackageSourceURL string
	ReadMe           safehtml.HTML
	ReadMeSource     string
	Redistributable  bool
	RepositoryURL    string
}

// fetchOverviewDetails uses the given version to fetch an OverviewDetails.
// versionedLinks says whether the constructed URLs should have versions.
func fetchOverviewDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, versionedLinks bool) (*OverviewDetails, error) {
	u, err := ds.GetUnit(ctx, um, internal.WithReadme)
	if err != nil {
		return nil, err
	}
	var readme *internal.Readme
	if u.Readme != nil {
		readme = &internal.Readme{Filepath: u.Readme.Filepath, Contents: u.Readme.Contents}
	}
	mi := &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
		SourceInfo:        u.SourceInfo,
	}
	return constructOverviewDetails(ctx, mi, readme, u.IsRedistributable, versionedLinks)
}

// constructOverviewDetails uses the given module version and readme to
// construct an OverviewDetails. versionedLinks says whether the constructed URLs should have versions.
func constructOverviewDetails(ctx context.Context, mi *internal.ModuleInfo, readme *internal.Readme, isRedistributable bool, versionedLinks bool) (*OverviewDetails, error) {
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
		r, err := ReadmeHTML(ctx, mi, readme)
		if err != nil {
			return nil, err
		}
		overview.ReadMe = r
	}
	return overview, nil
}

// fetchPackageOverviewDetails uses data for the given versioned directory to return an OverviewDetails.
func fetchPackageOverviewDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, versionedLinks bool) (*OverviewDetails, error) {
	od, err := fetchOverviewDetails(ctx, ds, um, versionedLinks)
	if err != nil {
		return nil, err
	}
	od.RepositoryURL = um.SourceInfo.RepoURL()
	od.PackageSourceURL = um.SourceInfo.DirectoryURL(internal.Suffix(um.Path, um.ModulePath))
	return od, nil
}

func blackfridayReadmeHTML(ctx context.Context, readme *internal.Readme, mi *internal.ModuleInfo) (safehtml.HTML, error) {
	// blackfriday.Run() uses CommonHTMLFlags and CommonExtensions by default.
	renderer := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{Flags: blackfriday.CommonHTMLFlags})
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions | blackfriday.AutoHeadingIDs))

	// Render HTML similar to blackfriday.Run(), but here we implement a custom
	// Walk function in order to modify image paths in the rendered HTML.
	b := &bytes.Buffer{}
	contents := bytes.ReplaceAll([]byte(readme.Contents), []byte("\r"), nil)
	rootNode := parser.Parse(contents)
	var walkErr error
	rootNode.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		switch node.Type {
		case blackfriday.Heading:
			if experiment.IsActive(ctx, internal.ExperimentUnitPage) && node.HeadingID != "" {
				// Prefix HeadingID with "readme-" on the unit page to prevent
				// a namespace clash with the documentation section.
				node.HeadingID = "readme-" + node.HeadingID
			}
		case blackfriday.Image, blackfriday.Link:
			useRaw := node.Type == blackfriday.Image
			if d := translateRelativeLink(string(node.LinkData.Destination), mi.SourceInfo, useRaw, readme); d != "" {
				node.LinkData.Destination = []byte(d)
			}
		case blackfriday.HTMLBlock, blackfriday.HTMLSpan:
			d, err := translateHTML(node.Literal, mi.SourceInfo, readme)
			if err != nil {
				walkErr = fmt.Errorf("couldn't transform html block(%s): %w", node.Literal, err)
				return blackfriday.Terminate
			}
			node.Literal = d
		}
		return renderer.RenderNode(b, node, entering)
	})
	if walkErr != nil {
		return safehtml.HTML{}, walkErr
	}
	return sanitizeHTML(b), nil
}

// ReadmeHTML sanitizes readmeContents based on bluemondy.UGCPolicy and returns
// a safehtml.HTML. If readmeFilePath indicates that this is a markdown file,
// it will also render the markdown contents using blackfriday.
//
// It is exported to support external testing.
func ReadmeHTML(ctx context.Context, mi *internal.ModuleInfo, readme *internal.Readme) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "readmeHTML(%s@%s)", mi.ModulePath, mi.Version)
	if readme == nil || readme.Contents == "" {
		return safehtml.HTML{}, nil
	}
	if !isMarkdown(readme.Filepath) {
		t := template.Must(template.New("").Parse(`<pre class="readme">{{.}}</pre>`))
		h, err := t.ExecuteToHTML(readme.Contents)
		if err != nil {
			return safehtml.HTML{}, err
		}
		return h, nil
	}

	if experiment.IsActive(ctx, internal.ExperimentGoldmark) {
		// Sets priority value so that we always use our custom transformer
		// instead of the default ones. The default values are in:
		// https://github.com/yuin/goldmark/blob/7b90f04af43131db79ec320be0bd4744079b346f/parser/parser.go#L567
		const ASTTransformerPriority = 10000
		gdMarkdown := goldmark.New(
			goldmark.WithParserOptions(
				// WithHeadingAttribute allows us to include other attributes in
				// heading tags. This is useful for our aria-level implementation of
				// increasing heading rankings.
				parser.WithHeadingAttribute(),
				// Generates an id in every heading tag. This is used in github in
				// order to generate a link with a hash that a user would scroll to
				// <h1 id="goldmark">goldmark</h1> => github.com/yuin/goldmark#goldmark
				parser.WithAutoHeadingID(),
				// Include custom ASTTransformer using the readme and module info to
				// use translateRelativeLink and translateHTML to modify the AST
				// before it is rendered.
				parser.WithASTTransformers(util.Prioritized(&ASTTransformer{
					info:   mi.SourceInfo,
					readme: readme,
				}, ASTTransformerPriority)),
			),
			// These extensions lets users write HTML code in the README. This is
			// fine since we process the contents using bluemonday after.
			goldmark.WithRendererOptions(goldmarkHtml.WithUnsafe(), goldmarkHtml.WithXHTML()),
			goldmark.WithExtensions(
				extension.GFM, // Support Github Flavored Markdown.
				emoji.Emoji,   // Support Github markdown emoji markup.
			),
		)
		gdMarkdown.Renderer().AddOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(NewHTMLRenderer(mi.SourceInfo, readme), 100),
			),
		)

		var b bytes.Buffer
		contents := []byte(readme.Contents)
		gdRenderer := gdMarkdown.Renderer()
		gdParser := gdMarkdown.Parser()

		reader := text.NewReader(contents)
		doc := gdParser.Parse(reader)

		if err := gdRenderer.Render(&b, contents, doc); err != nil {
			return safehtml.HTML{}, nil
		}

		return sanitizeGoldmarkHTML(&b), nil
	}
	return blackfridayReadmeHTML(ctx, readme, mi)
}

// sanitizeGoldmarkHTML sanitizes HTML from a bytes.Buffer so that it is safe.
func sanitizeGoldmarkHTML(b *bytes.Buffer) safehtml.HTML {
	p := bluemonday.UGCPolicy()

	p.AllowAttrs("width", "align").OnElements("img")
	p.AllowAttrs("width", "align").OnElements("div")
	p.AllowAttrs("width", "align").OnElements("p")
	// Allow accessible headings (i.e <div role="heading" aria-level="7">).
	p.AllowAttrs("width", "align", "role", "aria-level").OnElements("div")
	for _, h := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		// Needed to preserve github styles heading font-sizes
		p.AllowAttrs("class").OnElements(h)
	}

	s := string(p.SanitizeBytes(b.Bytes()))
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s)
}

// sanitizeHTML reads HTML from r and sanitizes it to ensure it is safe.
func sanitizeHTML(r io.Reader) safehtml.HTML {
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
	s := p.SanitizeReader(r).String()
	// Trust that bluemonday properly sanitizes the HTML.
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s)
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
func translateRelativeLink(dest string, info *source.Info, useRaw bool, readme *internal.Readme) string {
	destURL, err := url.Parse(dest)
	if err != nil || destURL.IsAbs() {
		return ""
	}
	if destURL.Path == "" {
		// This is a fragment; leave it.
		return ""
	}
	// Paths are relative to the README location.
	destPath := path.Join(path.Dir(readme.Filepath), path.Clean(trimmedEscapedPath(destURL)))
	if useRaw {
		return info.RawURL(destPath)
	}
	return info.FileURL(destPath)
}

// trimmedEscapedPath trims surrounding whitespace from u's path, then returns it escaped.
func trimmedEscapedPath(u *url.URL) string {
	u.Path = strings.TrimSpace(u.Path)
	return u.EscapedPath()
}

// translateHTML parses html text into parsed html nodes. It then
// iterates through the nodes and replaces the src key with a value
// that properly represents the source of the image from the repo.
func translateHTML(htmlText []byte, info *source.Info, readme *internal.Readme) (_ []byte, err error) {
	defer derrors.Wrap(&err, "translateHTML(readme.Filepath=%s)", readme.Filepath)

	r := bytes.NewReader(htmlText)
	nodes, err := html.ParseFragment(r, nil)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	changed := false
	for _, n := range nodes {
		// We expect every parsed node to begin with <html><head></head><body>.
		if n.DataAtom != atom.Html {
			return nil, fmt.Errorf("top-level node is %q, expected 'html'", n.DataAtom)
		}
		// When the parsed html nodes don't have a valid structure
		// (i.e: an html comment), then just return the original text.
		if n.FirstChild == nil || n.FirstChild.NextSibling == nil || n.FirstChild.NextSibling.DataAtom != atom.Body {
			return htmlText, nil
		}
		n = n.FirstChild.NextSibling
		// n is now the body node. Walk all its children.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walkHTML(c, info, readme) {
				changed = true
			}
			if err := html.Render(&buf, c); err != nil {
				return nil, err
			}
		}
	}
	if changed {
		return buf.Bytes(), nil
	}
	// If there were no changes, return the original.
	return htmlText, nil
}

// walkHTML crawls through an html node and replaces the src
// tag link with a link that properly represents the image
// from the repo source.
// It reports whether it made a change.
func walkHTML(n *html.Node, info *source.Info, readme *internal.Readme) bool {
	changed := false
	if n.Type == html.ElementNode && n.DataAtom == atom.Img {
		var attrs []html.Attribute
		for _, a := range n.Attr {
			if a.Key == "src" {
				if v := translateRelativeLink(a.Val, info, true, readme); v != "" {
					a.Val = v
					changed = true
				}
			}
			attrs = append(attrs, a)
		}
		n.Attr = attrs
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if walkHTML(c, info, readme) {
			changed = true
		}
	}
	return changed
}
