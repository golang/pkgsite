// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quote collects pithy sayings.
package quote // import "rsc.io/quote"

import "rsc.io/sampler"

// Hello returns a greeting.
func Hello() string {
	return sampler.Hello()
}

// Glass returns a useful phrase for world travelers.
func Glass() string {
	// See http://www.oocities.org/nodotus/hbglass.html.
	return "I can eat glass and it doesn't hurt me."
}

// Go returns a Go proverb.
func Go() string {
	return "Don't communicate by sharing memory, share memory by communicating."
}

// Opt returns an optimization truth.
func Opt() string {
	// Wisdom from ken.
	return "If a program is too slow, it must have a loop."
}
