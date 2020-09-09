// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command css appends CSS styles to content/static/stylesheet.css.
// It reads from the CSS file at
// https://github.com/sindresorhus/github-markdown-css/blob/gh-pages/github-markdown.css
// and removes all styles that do not belong to a .markdown-body <tag>.  The
// .markdown-body class is then replaced with .Overview-readmeContent, for use
// in the discovery codebase. The remaining properties are written to content/static/css/stylesheet.css.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	cssFile              = "content/static/stylesheet.css"
	githubStylesheet     = "https://raw.githubusercontent.com/sindresorhus/github-markdown-css/gh-pages/github-markdown.css"
	githubREADMEClass    = ".markdown-body"
	discoveryREADMEClass = ".Overview-readmeContent"
)

var write = flag.Bool("write", false, "append modifications to content/static/css/stylesheet.css")

func main() {
	flag.Parse()

	resp, err := http.Get(githubStylesheet)
	if err != nil {
		log.Fatalf("http.Get(%q): %v", githubStylesheet, err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("http.Get(%q): status = %d", githubStylesheet, resp.StatusCode)
	}

	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	var (
		atPropertyStart = true
		curr            string
		includeProperty bool
		properties      []string
	)
	for scanner.Scan() {
		text := scanner.Text()
		if atPropertyStart && shouldIncludeProperty(text) {
			includeProperty = true
		}
		if text == "}" {
			if includeProperty {
				properties = append(properties, curr+text+"\n")
			}
			curr = ""
			includeProperty = false
			atPropertyStart = true
			continue
		}
		if includeProperty {
			curr += text
			curr += "\n"
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile(cssFile, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("os.OpenFile(f, os.O_WRONLY|os.O_APPEND, 0644): %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Fatalf("file.Close(): %v", err)
		}
	}()

	if !*write {
		fmt.Println("Dryrun only. Run with `-write` to write to stylesheet.css.")
	} else {
		fmt.Printf("Writing these properties to %q: \n", cssFile)
	}

	contentsToWrite := `
/* ---------- */
/*
/* The CSS classes below are generated using content/static/css/main.go
/* To update, delete the contents below and and run go run content/static/css/main.go
/*
/* ---------- */`
	contentsToWrite += "\n\n"

	for _, p := range properties {
		contentsToWrite += strings.ReplaceAll(p, githubREADMEClass, discoveryREADMEClass)
	}

	contentsToWrite += `
/* ---------- */
/*
/* End output from content/static/css/main.go.
/*
/* ---------- */`

	fmt.Println(contentsToWrite)

	if _, err := file.WriteString(contentsToWrite); err != nil {
		log.Fatalf("file.WriteString(%q): %v", contentsToWrite, err)
	}
}

// shouldIncludeProperty reports whether this property should be included in
// the CSS file.
func shouldIncludeProperty(property string) bool {
	parts := strings.Split(property, " ")
	if len(parts) < 1 {
		return false
	}
	if parts[0] != githubREADMEClass {
		return false
	}
	for _, p := range parts[1:] {
		if strings.HasPrefix(p, ".") {
			return false
		}
	}
	return true
}
