// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/russross/blackfriday/v2"
)

const (
	maxSectionWords   = 50
	maxReadmeFraction = 0.5
)

// SearchDocumentSections computes the B and C sections of a Postgres search
// document from a package synopsis and a README.
// By "B section" and "C section" we mean the portion of the tsvector with weight
// "B" and "C", respectively.
//
// The B section consists of the synopsis.
// The C section consists of the first sentence of the README.
// The D section consists of the remainder of the README.
// All sections are split into words and processed for replacements.
// Each section is limited to maxSectionWords words, and in addition the
// D section is limited to an initial fraction of the README, determined
// by maxReadmeFraction.
func SearchDocumentSections(synopsis, readmeFilename, readme string) (b, c, d string) {
	return searchDocumentSections(synopsis, readmeFilename, readme, maxSectionWords, maxReadmeFraction)
}

func searchDocumentSections(synopsis, readmeFilename, readme string, maxSecWords int, maxReadmeFrac float64) (b, c, d string) {
	var readmeFirst, readmeRest string
	if isMarkdown(readmeFilename) {
		readme = processMarkdown(readme)
	}
	if i := sentenceEndIndex(readme); i > 0 {
		readmeFirst, readmeRest = readme[:i+1], readme[i+1:]
	} else {
		readmeRest = readme
	}
	sw := processWords(synopsis)
	rwf := processWords(readmeFirst)
	rwr := processWords(readmeRest)

	sectionB, _ := split(sw, maxSecWords)
	sectionC, rwfd := split(rwf, maxSecWords)
	// section D is the part of the readme that is not in sectionC.
	rwd := append(rwfd, rwr...)
	// Keep maxSecWords of section D, but not more than maxReadmeFrac.
	f := int(maxReadmeFrac * float64(len(rwd)))
	nkeep := maxSecWords
	if nkeep > f {
		nkeep = f
	}
	sectionD, _ := split(rwd, nkeep)

	// If there is no synopsis, use first sentence of the README.
	// But do not promote the rest of the README to section C.
	if len(sectionB) == 0 {
		sectionB = sectionC
		sectionC = nil
	}

	prep := func(ws []string) string {
		return makeValidUnicode(strings.Join(ws, " "))
	}

	return prep(sectionB), prep(sectionC), prep(sectionD)
}

// split splits a slice of strings into two parts. The first has length <= n,
// and the second is the rest of the slice. If n is negative, the first part is nil and
// the second part is the entire slice.
func split(a []string, n int) ([]string, []string) {
	if n >= len(a) {
		return a, nil
	}
	return a[:n], a[n:]
}

// sentenceEndIndex returns the index in s of the end of the first sentence, or
// -1 if no end can be found. A sentence ends at a '.', '!' or '?' that is
// followed by a space (or ends the string), and is not preceded by an
// uppercase letter.
func sentenceEndIndex(s string) int {
	var prev1, prev2 rune

	end := func() bool {
		return !unicode.IsUpper(prev2) && (prev1 == '.' || prev1 == '!' || prev1 == '?')
	}

	for i, r := range s {
		if unicode.IsSpace(r) && end() {
			return i - 1
		}
		prev2 = prev1
		prev1 = r
	}
	if end() {
		return len(s) - 1
	}
	return -1
}

// processWords splits s into words at whitespace, then processes each word.
func processWords(s string) []string {
	fields := strings.Fields(strings.ToLower(s))
	var words []string
	for _, f := range fields {
		words = append(words, processWord(f)...)
	}
	return words
}

// summaryReplacements is used to replace words with other words.
// It is used by processWord, below.
// Example key-value pairs:
//
//	"deleteMe": nil					 // removes "deleteMe"
//	"rand": []string{"random"}			 // replace "rand" with "random"
//	"utf-8": []string{"utf-8", "utf8"}  // add "utf8" whenever "utf-8" is seen
var summaryReplacements = map[string][]string{
	"postgres":   {"postgres", "postgresql"},
	"postgresql": {"postgres", "postgresql"},
	"rand":       {"random"},
	"mongo":      {"mongo", "mongodb"},
	"mongodb":    {"mongo", "mongodb"},
	"redis":      {"redis", "redisdb"},
	"redisdb":    {"redis", "redisdb"},
	"logger":     {"logger", "log"}, // Postgres stemmer does not handle -er
	"parser":     {"parser", "parse"},
	"utf-8":      {"utf-8", "utf8"},
}

// processWord performs processing on s, returning zero or more words.
// Its main purpose is to apply summaryReplacements to replace
// certain words with synonyms or additional search terms.
func processWord(s string) []string {
	s = strings.TrimFunc(s, unicode.IsPunct)
	if s == "" {
		return nil
	}
	if rs, ok := summaryReplacements[s]; ok {
		return rs
	}
	if !hyphenSplit(s) {
		return []string{s}
	}
	// Apply replacements to parts of hyphenated words.
	ws := strings.Split(s, "-")
	if len(ws) == 1 {
		return ws
	}
	result := []string{s} // Include the full hyphenated word.
	for _, w := range ws {
		if rs, ok := summaryReplacements[w]; ok {
			result = append(result, rs...)
		}
		// We don't need to include the parts; the Postgres text-search processor will do that.
	}
	return result
}

// hyphenSplit reports whether s should be split on hyphens.
func hyphenSplit(s string) bool {
	return !(strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"))
}

// isMarkdown reports whether filename says that the file contains markdown.
func isMarkdown(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	// https://tools.ietf.org/html/rfc7763 mentions both extensions.
	return ext == ".md" || ext == ".markdown"
}

// processMarkdown returns the text of a markdown document.
// It omits all formatting and images.
func processMarkdown(s string) string {
	parser := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions))
	root := parser.Parse([]byte(s))
	buf := walkMarkdown(root, nil, 0)
	return string(buf)
}

// walkMarkdown traverses a blackfriday parse tree, extracting text.
func walkMarkdown(n *blackfriday.Node, buf []byte, level int) []byte {
	if n == nil {
		return buf
	}
	switch n.Type {
	case blackfriday.Image:
		// Skip images because they usually are irrelevant to the package
		// (badges and such).
		return buf
	case blackfriday.CodeBlock:
		// Skip code blocks because they have a wide variety of unrelated symbols.
		return buf
	case blackfriday.Paragraph, blackfriday.Heading:
		if len(buf) > 0 {
			buf = append(buf, ' ')
		}
	default:
		buf = append(buf, n.Literal...)
	}
	for c := n.FirstChild; c != nil; c = c.Next {
		buf = walkMarkdown(c, buf, level+1)
	}
	return buf
}
