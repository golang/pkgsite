// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doc

import (
	"unicode"
	"unicode/utf8"
)

// token۰IsExported reports whether name starts with an upper-case letter.
//
func token۰IsExported(name string) bool {
	ch, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(ch)
}
