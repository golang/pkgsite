// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package example_test has tests for the example code in given package
// It is used to test that the playground example's generated HTML has
// code with imports in a executable examples and without in a
// non executable one.
package example_test

import (
	"fmt"
	"strings"
)

// non-executable example taken from https://github.com/urfave/cli/blob/master/app_test.go#L184
func Example_appRunNoAction() {
	// example comment
	app := App{}
	app.Name = "greet"
	_ = app.Run([]string{"greet"})

	// Output:
	// NAME:
	//    greet - A new cli application
	//
	// USAGE:
	//    greet [global options] command [command options] [arguments...]
	//
	// COMMANDS:
	//    help, h  Shows a list of commands or help for one command
	//
	// GLOBAL OPTIONS:
	//    --help, -h  show help (default: false)
}

// executable example
func Example_stringsCompare() {
	// example comment
	fmt.Println(strings.Compare("a", "b"))
	fmt.Println(strings.Compare("a", "a"))
	fmt.Println(strings.Compare("b", "a"))

	// Output:
	// -1
	// 0
	// 1
}
