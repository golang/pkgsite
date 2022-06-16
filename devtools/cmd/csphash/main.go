// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// csphash computes the hashes of script tags in files,
// and checks that they are added to our content
// security policy.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
)

var hashFile = flag.String("hf", "internal/middleware/secureheaders.go", "file with hashes for CSP header")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] FILES\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "suggestion for FILES: content/static/html/**/*.tmpl\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	cspHashes, err := extractHashes(*hashFile)
	if err != nil {
		log.Fatal(err)
	}
	cspHashMap := map[string]bool{}
	for _, h := range cspHashes {
		cspHashMap[h] = true
	}

	ok := true
	for _, file := range flag.Args() {
		scripts, err := scripts(file)
		if err != nil {
			log.Fatal(err)
		}
		for _, s := range scripts {
			if bytes.Contains(s.tag, []byte("src=")) {
				fmt.Printf("%s: has script with src attribute: %s\n", file, s.tag)
				ok = false
			}
			if bytes.Contains(s.body, []byte("{{")) {
				fmt.Printf("%s: has script with template expansion:\n%s\n", file, s.body)
				fmt.Printf("Scripts must be static so they have a constant hash.\n")
				ok = false
				continue
			}
			hash := cspHash(s.body)
			if !cspHashMap[hash] {
				fmt.Printf("missing hash: add the lines below to %s:\n", *hashFile)
				fmt.Printf("    // From %s\n", file)
				fmt.Printf(`    "'sha256-%s'",`, hash)
				fmt.Println()
				ok = false
			} else {
				delete(cspHashMap, hash)
			}
		}
	}
	for h := range cspHashMap {
		fmt.Printf("unused hash %s\n", h)
		ok = false
	}
	if !ok {
		fmt.Printf("Add missing hashes to %s and remove unused ones.\n", *hashFile)
		os.Exit(1)
	}
}

var hashRegexp = regexp.MustCompile(`'sha256-([^']+)'`)

// extractHashes scans the given file for CSP-style hashes and returns them.
func extractHashes(filename string) ([]string, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var hashes []string
	matches := hashRegexp.FindAllSubmatch(contents, -1)
	for _, m := range matches {
		hashes = append(hashes, string(m[1]))
	}
	return hashes, nil
}

func cspHash(b []byte) string {
	h := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(h[:])
}

// script represents an HTML script element.
type script struct {
	tag  []byte // `<script attr="a"...>`
	body []byte // text between open and close script tags
}

// scripts returns all the script elements in the given file.
func scripts(filename string) ([]*script, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	return scriptsReader(contents)
}

var (
	// Assume none of the attribute values contain a '>'.
	// Regexp flag `i` means case-insensitive.
	scriptStartRegexp = regexp.MustCompile(`(?i:<script>|<script\s[^>]*>)`)
	// Assume all scripts end with a full close tag.
	scriptEndRegexp = regexp.MustCompile(`(?i:</script>)`)
)

func scriptsReader(b []byte) ([]*script, error) {
	var scripts []*script
	offset := 0
	for {
		start := scriptStartRegexp.FindIndex(b)
		if start == nil {
			return scripts, nil
		}
		tag := b[start[0]:start[1]]
		b = b[start[1]:]
		offset += start[1]
		end := scriptEndRegexp.FindIndex(b)
		if end == nil {
			return nil, fmt.Errorf("%s is missing an end tag", tag)
		}
		scripts = append(scripts, &script{
			tag:  tag,
			body: b[:end[0]],
		})
		b = b[end[1]:]
		offset += end[1]
	}
}
