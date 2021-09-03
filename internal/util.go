// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bufio"
	"os"
	"strings"

	"golang.org/x/pkgsite/internal/derrors"
)

// Readfilelines reads and returns the lines from a file.
// Whitespace on each line is trimmed.
// Blank lines and lines beginning with '#' are ignored.
func ReadFileLines(filename string) (lines []string, err error) {
	defer derrors.Wrap(&err, "ReadFileLines(%q)", filename)

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if s.Err() != nil {
		return nil, s.Err()
	}
	return lines, nil
}

// Cut cuts s around the first instance of sep,
// returning the text before and after sep.
// The found result reports whether sep appears in s.
// If sep does not appear in s, cut returns s, "", false.
//
// https://golang.org/issue/46336 is an accepted proposal to add this to the
// standard library. It will presumably land in Go 1.18, so this can be removed
// when pkgsite moves to that version.
func Cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}
