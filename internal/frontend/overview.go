// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/source"
)

// isMarkdown reports whether filename says that the file contains markdown.
func isMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	// https://tools.ietf.org/html/rfc7763 mentions both extensions.
	return ext == ".md" || ext == ".markdown"
}

// translateLink converts image links so that they will work on pkgsite.
//
// README files sometimes use relative image paths to image files inside the
// repository. As the discovery site doesn't host the full repository content,
// in order for the image to render, we need to convert the relative path to an
// absolute URL to a hosted image.
//
// In addition, GitHub will translate absolute non-raw links to image files to raw links.
// For example, when GitHub renders a README with
//
//	<img src="https://github.com/gobuffalo/buffalo/blob/master/logo.svg">
//
// it rewrites it to
//
//	<img src="https://github.com/gobuffalo/buffalo/raw/master/logo.svg">
//
// (replacing "blob" with "raw").
// We do that too.
func translateLink(dest string, info *source.Info, useRaw bool, readme *internal.Readme) string {
	destURL, err := url.Parse(dest)
	if err != nil {
		return ""
	}
	if destURL.IsAbs() {
		if destURL.Host != "github.com" {
			return ""
		}
		if strings.HasSuffix(destURL.Path, ".md") {
			return ""
		}
		parts := strings.Split(destURL.Path, "/")
		if len(parts) < 4 || parts[3] != "blob" {
			return ""
		}
		parts[3] = "raw"
		destURL.Path = strings.Join(parts, "/")
		return destURL.String()
	}
	if destURL.Path == "" {
		// This is a fragment; leave it.
		return "#readme-" + destURL.Fragment
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
				if v := translateLink(a.Val, info, true, readme); v != "" {
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
