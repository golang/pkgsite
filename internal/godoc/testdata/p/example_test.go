// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p_test

import "golang.org/x/pkgsite/internal/godoc/testdata/p"

// non-executable example
func ExampleTF() {
	// example comment
	app := App{}
	app.Name = "greet"
	_ = app.Run([]string{"greet"})
}

// executable example
func ExampleF() {
	// example comment
	p.F()
	// Output:
	// ok
}
