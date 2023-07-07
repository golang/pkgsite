// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Since github.com/evanw/esbuild doesn't build on plan9
// this function stubs out the build function with a function
// that panics.

//go:build plan9

package static

func Build(config Config) error {
	panic("This functionality is not supported on plan9")
}
