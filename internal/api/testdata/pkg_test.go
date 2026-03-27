// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"fmt"

	"golang.org/x/pkgsite/internal/api/testdata"
)

func Example() {
	fmt.Println("Package example")
	// Output:
	// Package example
}

func ExampleF() {
	pkg.F()
	fmt.Println("F example")
	// Output:
	// F example
}

func ExampleF_second() {
	pkg.F()
	fmt.Println("F second example")
	// Output:
	// F second example
}

func ExampleT() {
	_ = pkg.T(0)
	fmt.Println("T example")
	// Output:
	// T example
}

func ExampleT_M() {
	var t pkg.T
	t.M()
	fmt.Println("M example")
	// Output:
	// M example
}
