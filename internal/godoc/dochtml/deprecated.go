// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"go/doc"
	"regexp"
)

// "Deprecated:" at the start of a paragraph.
var deprecatedRx = regexp.MustCompile(`(^|\n\s*\n)\s*Deprecated:`)

// IsDeprecated reports whether the string has a "Deprecated" line.
func IsDeprecated(s string) bool {
	return deprecatedRx.MatchString(s)
}

func typeIsDeprecated(t *doc.Type) bool {
	return IsDeprecated(t.Doc)
}

func valueIsDeprecated(v *doc.Value) bool {
	return IsDeprecated(v.Doc)
}

func funcIsDeprecated(f *doc.Func) bool {
	return IsDeprecated(f.Doc)
}
