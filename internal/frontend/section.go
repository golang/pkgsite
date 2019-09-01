// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"strings"
)

// A Section represents a collection of lines with a common prefix. The
// collection is itself divided into sections by prefix, forming a tree.
type Section struct {
	Prefix   string     // prefix for this section, or if Subs==nil, a single line
	Subs     []*Section // subsections
	NumLines int        // total number of lines in subsections
}

func newSection(prefix string) *Section {
	return &Section{Prefix: prefix, NumLines: 0}
}

func (s *Section) add(sub *Section) {
	s.Subs = append(s.Subs, sub)
	if sub.Subs == nil {
		s.NumLines++
	} else {
		s.NumLines += sub.NumLines
	}
}

// A prefixFunc returns the next prefix of s, given the current prefix.
// It should return the empty string if there are no more prefixes.
type prefixFunc func(s, prefix string) string

// Sections transforms a list of lines, which must be sorted, into a list
// of Sections. Each Section in the result contains all the contiguous lines
// with the same prefix.
//
// The nextPrefix function is responsible for extracting prefixes from lines.
func Sections(lines []string, nextPrefix prefixFunc) []*Section {
	s, _ := section("", lines, nextPrefix)
	return s.Subs
}

// section collects all lines with the same prefix into a section. It assumes
// that lines is sorted. It returns the section along with the remaining lines.
func section(prefix string, lines []string, nextPrefix prefixFunc) (*Section, []string) {
	s := newSection(prefix)
	for len(lines) > 0 {
		l := lines[0]
		if !strings.HasPrefix(l, prefix) {
			break
		}

		np := nextPrefix(l, prefix)
		var sub *Section
		if np == "" {
			sub = newSection(l)
			lines = lines[1:]
		} else {
			sub, lines = section(np, lines, nextPrefix)
		}
		s.add(sub)
	}
	// Collapse a section with a single subsection, except at top level.
	if len(s.Subs) == 1 && prefix != "" {
		s = s.Subs[0]
	}
	return s, lines
}

// nextPrefixAccount is a prefixFunc (see above). Its first argument
// is an import path, and its second is the previous prefix that it returned
// for that path, or "" if this is the first prefix.
//
// nextPrefixAccount tries to return an initial prefix for path
// that consists of the "account": the entity controlling the
// remainder of the path. In the most common case, paths beginning
// with "github.com", the account is the second path element, the GitHub user or org.
// So for example, the first prefix of "github.com/google/go-cmp/cmp" is
// "github.com/google/".
//
// nextPrefixAccount returns a second prefix that is one path element past the
// account. For github.com paths, this is the repo. Continuing the above example,
// the second prefix is "github.com/google/go-cmp/".
//
// nextPrefixAccount does not return any prefixes beyond those two.
func nextPrefixAccount(path, pre string) string {
	// If the last prefix consisted of the entire path, then
	// there is no next prefix.
	if path == pre {
		return ""
	}
	parts := strings.Split(path, "/")
	prefix1, acctParts := accountPrefix(parts)
	if pre == "" {
		return prefix1
	}
	if pre == prefix1 {
		// Second prefix: one element past the first.
		// The +1 is safe because we know that pre is shorter than path from
		// the first test of the function.
		prefix2 := strings.Join(parts[:len(acctParts)+1], "/")
		if prefix2 != path {
			prefix2 += "/"
		}
		return prefix2
	}
	// No more prefixes after the first two.
	return ""
}

// accountPrefix guesses the prefix of the path (split into parts at "/")
// that corresponds to the account.
func accountPrefix(parts []string) (string, []string) {
	// TODO(jba): handle repo import paths like "example.org/foo/bar.hg".
	var n int // index of account in parts
	// The first two cases below handle the special cases that the go command does.
	// See "go help importpath".
	switch parts[0] {
	case "github.com", "bitbucket.org", "launchpad.net", "golang.org":
		n = 1
	case "hub.jazz.net":
		n = 2
	default:
		// For custom import paths, use the host as the first prefix.
		n = 0
	}
	if n >= len(parts)-1 {
		return strings.Join(parts, "/"), parts
	}
	return strings.Join(parts[:n+1], "/") + "/", parts[:n+1]
}
