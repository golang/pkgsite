// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package complete defines a Completion type that is used in auto-completion,
// along with Encode and Decode methods that can be used for storing this type
// in redis.
package complete

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/pkgsite/internal/derrors"
)

const keySep = "|"

// Redis keys for completion sorted sets ("indexes"). They are in this package
// so that they can be accessed by both worker and frontend.
const (
	KeyPrefix    = "completions"
	PopularKey   = KeyPrefix + "Popular"
	RemainingKey = KeyPrefix + "Rest"
)

// Completion holds package data from an auto-completion match.
type Completion struct {
	// Suffix is the path suffix that matched the compltion input, e.g. a query
	// for "error" would match the suffix "errors" of "github.com/pkg/errors".
	Suffix string
	// ModulePath is the module path of the completion match. We may support
	// matches of the same path in different modules.
	ModulePath string
	// Version is the module version of the completion entry.
	Version string
	// PackagePath is the full import path.
	PackagePath string
	// Importers is the number of importers of this package. It is used for
	// sorting completion results.
	Importers int
}

// Encode string-encodes a completion for storing in the completion index.
func (c Completion) Encode() string {
	return strings.Join(c.keyData(), keySep)
}

func (c Completion) keyData() []string {
	var suffix string
	if strings.HasPrefix(c.PackagePath, c.ModulePath) {
		suffix = strings.TrimPrefix(c.PackagePath, c.ModulePath)
		suffix = "/" + strings.Trim(suffix, "/")
	} else {
		// In the case of the standard library, ModulePath will not be a prefix of
		// PackagePath.
		suffix = c.PackagePath
	}
	return []string{
		c.Suffix,
		strings.TrimRight(c.ModulePath, "/"),
		c.Version,
		suffix,
		// It's important that importers is last in this key, since it is the only
		// datum that changes. By having it last, we reserve the ability to
		// selectively update this entry by deleting the prefix corresponding to
		// the values above.
		strconv.Itoa(c.Importers),
	}
}

// Decode parses a completion entry from the completions index.
func Decode(entry string) (_ *Completion, err error) {
	defer derrors.Wrap(&err, "complete.Decode(%q)", entry)
	parts := strings.Split(entry, "|")
	if len(parts) != 5 {
		return nil, fmt.Errorf("got %d parts, want 5", len(parts))
	}
	c := &Completion{
		Suffix:     parts[0],
		ModulePath: parts[1],
		Version:    parts[2],
	}
	suffix := parts[3]
	if strings.HasPrefix(suffix, "/") {
		c.PackagePath = strings.Trim(c.ModulePath+suffix, "/")
	} else {
		c.PackagePath = suffix
	}
	importers, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, fmt.Errorf("error parsing importers: %v", err)
	}
	c.Importers = importers
	return c, nil
}

// PathCompletions generates completion entries for all possible suffixes of
// partial.PackagePath.
func PathCompletions(partial Completion) []*Completion {
	suffs := pathSuffixes(partial.PackagePath)
	var cs []*Completion
	for _, pref := range suffs {
		var next = partial
		next.Suffix = pref
		cs = append(cs, &next)
	}
	return cs
}

// pathSuffixes returns a slice of all path suffixes of a '/'-separated path,
// including the full path itself. i.e.
//   pathSuffixes("foo/bar") = []string{"foo/bar", "bar"}
func pathSuffixes(path string) []string {
	path = strings.ToLower(path)
	var prefs []string
	for len(path) > 0 {
		prefs = append(prefs, path)
		i := strings.Index(path, "/")
		if i < 0 {
			break
		}
		path = path[i+1:]
	}
	return prefs
}
