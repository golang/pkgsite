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
	"regexp"
	"strconv"
	"strings"
)

const (
	cssFile              = "static/frontend/unit/main/_readme_gen.css"
	githubStylesheet     = "https://raw.githubusercontent.com/sindresorhus/github-markdown-css/gh-pages/github-markdown.css"
	githubREADMEClass    = ".markdown-body"
	discoveryREADMEClass = ".Overview-readmeContent"
	copyright            = `/*
* Copyright 2019-2020 The Go Authors. All rights reserved.
* Use of this source code is governed by a BSD-style
* license that can be found in the LICENSE file.
*/
`
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
		if headerString := replaceHeaderTag(text); headerString != "" {
			text = headerString
		}
		if atPropertyStart && shouldIncludeProperty(text) {
			includeProperty = true
		}
		if remString := replaceValueWithRems(text); remString != "" {
			text = remString
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

	if err := os.WriteFile(cssFile, []byte(copyright), 0644); err != nil {
		log.Fatalf("os.WriteFile(f, '', 0644): %v", err)
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
/* The CSS classes below are generated using devtools/cmd/css/main.go
/* If the generated CSS already exists, the file is overwritten
/*
/* ---------- */`
	contentsToWrite += "\n\n"

	for _, p := range properties {
		contentsToWrite += strings.ReplaceAll(p, githubREADMEClass, discoveryREADMEClass)
	}

	contentsToWrite += `
/* ---------- */
/*
/* End output from devtools/cmd/css/main.go
/*
/* ---------- */`

	fmt.Println(contentsToWrite)

	if _, err := file.WriteString(contentsToWrite); err != nil {
		log.Fatalf("file.WriteString(%q): %v", contentsToWrite, err)
	}
}

// replaceHeaderTag finds any header tags in a line of text and increases
// the header level by 2. replaceHeader tag returns the replaced string if a
// header tag is found and returns an empty string if not
func replaceHeaderTag(property string) string {
	headerMap := map[string]string{
		"h1": "h3", "h2": "h4", "h3": "h5", "h4": "h6", "h5": "div[aria-level=7]", "h6": "div[aria-level=8]",
	}
	for k, v := range headerMap {
		if strings.Contains(property, k) {
			return strings.ReplaceAll(property, k, v)
		}
	}
	return ""
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

// pxToRem returns the number value of a px string to a rem string.
func pxToRem(value string) string {
	valueNum, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return ""
	}
	valueNum = valueNum / 16
	return fmt.Sprintf("%frem", valueNum)
}

// replaceValueWithRems replaces the px values in a line of css with rems.
// e.g: padding: 25px 10px => padding:
func replaceValueWithRems(line string) string {
	var cssLine string
	valueRegex := regexp.MustCompile(`([-+]?[0-9]*\.?[0-9]+)px`)
	matches := valueRegex.FindAllStringSubmatchIndex(line, -1)
	for idx, m := range matches {
		// e.g: "padding: 6px 13px;" => "padding: 0.375rem 0.8125rem;"
		//       padding: [valueStartIdx][numStartIdx]25[numEndIdx]px[valueEndIdx] 10em;
		// The value here is the full string "25px" and num is just "25".
		valueStartIdx, valueEndIdx, numStartIdx, numEndIdx := m[0], m[1], m[2], m[3]
		if idx == 0 {
			cssLine += line[0:valueStartIdx]
		}
		cssLine += pxToRem(line[numStartIdx:numEndIdx])
		if idx == len(matches)-1 {
			cssLine += line[valueEndIdx:]
		} else {
			// If there are more matches for "px", add up until the start of the next match.
			cssLine += line[valueEndIdx:matches[idx+1][0]]
		}
	}
	return cssLine
}
