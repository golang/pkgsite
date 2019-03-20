// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package foo
package foo

import (
	"fmt"
	"my/module/bar"
)

// FooBar returns the string "foo bar".
func FooBar() string {
	return fmt.Sprintf("foo %s", bar.Bar())
}
