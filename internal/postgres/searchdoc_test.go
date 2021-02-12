// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSearchDocumentSections(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                string
		synopsis            string
		readmeFilename      string
		readmeContents      string
		wantB, wantC, wantD string
	}{
		{
			"blackfriday",
			"This is a synopsis.",
			"foo.md",
			`Package blackfriday is a [markdown](http://foo) processor. That _is_ all that it is.`,

			"this is a synopsis",
			"package blackfriday is a markdown processor",
			"that is all",
		},
		{
			"non-markdown",
			"This synopsis is too long so we'll truncate it.",
			"README",
			"This README doesn't have a sentence end so the whole thing is D",

			"this synopsis is too long so",
			"",
			"this readme doesn't have a sentence",
		},
		{
			"viper",
			"",
			"README.md",
			`
![viper logo](https://cloud.githubusercontent.com/assets/173412/10886745/998df88a-8151-11e5-9448-4736db51020d.png)

Go configuration with fangs!

[![Actions](https://github.com/spf13/viper/workflows/CI/badge.svg)](https://github.com/spf13/viper)
[![Join the chat at https://gitter.im/spf13/viper](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/spf13/viper?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)
[![GoDoc](https://godoc.org/github.com/spf13/viper?status.svg)](https://godoc.org/github.com/spf13/viper)

Many Go projects are built using Viper including:`,

			"go configuration with fangs", // first sentence of README promoted
			"",
			"many go projects are",
		},
	} {
		gotB, gotC, gotD := searchDocumentSections(test.synopsis, test.readmeFilename, test.readmeContents, 6, 0.5)
		if gotB != test.wantB {
			t.Errorf("%s, B: got %q, want %q", test.name, gotB, test.wantB)
		}
		if gotC != test.wantC {
			t.Errorf("%s, C: got %q, want %q", test.name, gotC, test.wantC)
		}
		if gotD != test.wantD {
			t.Errorf("%s, D: got %q, want %q", test.name, gotD, test.wantD)
		}
	}
}

func TestProcessWords(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"foo", []string{"foo"}},
		{" foo \t bar\n", []string{"foo", "bar"}},
		{"http://foo/bar/baz?x=1", []string{"http://foo/bar/baz?x=1"}},
		{"This, however, shall. not; stand?", []string{"this", "however", "shall", "not", "stand"}},
		{"a postgres and NATS server over HTTP", []string{
			"a", "postgres", "postgresql", "and", "nats", "server", "over", "http"}},
		{"http://a-b-c.com full-text chart-parser", []string{
			"http://a-b-c.com", "full-text", "chart-parser", "parser", "parse"}},
	} {
		got := processWords(test.in)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%q:\ngot  %#v\nwant %#v", test.in, got, test.want)
		}
	}
}

func TestProcessMarkdown(t *testing.T) {
	t.Parallel()
	const (
		in = `
Blackfriday [![Build Status](https://travis-ci.org/russross/blackfriday.svg?branch=master)](https://travis-ci.org/russross/blackfriday)
===========

_Blackfriday_ is a [Markdown][1] *processor* implemented in [Go](https://golang.org).

[1]: https://daringfireball.net/projects/markdown/ "Markdown"
`

		want = `Blackfriday  Blackfriday is a Markdown processor implemented in Go.`
	)

	got := processMarkdown(in)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSentenceEndIndex(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want int
	}{
		{"", -1},
		{"Hello. What's up?", 5},
		{"unicode π∆!", 13},
		{"D. C. Fontana?", 13},
		{"D. c. Fontana?", 4},
		{"no end", -1},
	} {
		got := sentenceEndIndex(test.in)
		if got != test.want {
			t.Errorf("%s: got %d, want %d", test.in, got, test.want)
		}
	}
}
