// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package middleware implements a simple middleware pattern for http handlers,
// along with implementations for some common middlewares.
package middleware

import "net/http"

// A Middleware is a func that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain creates a new Middleware that applies a sequence of Middlewares, so
// that they execute in the given order when handling an http request.
//
// In other words, Chain(m1, m2)(handler) = m1(m2(handler))
//
// A similar pattern is used in e.g. github.com/justinas/alice:
// https://github.com/justinas/alice/blob/ce87934/chain.go#L45
func Chain(middlewares ...Middleware) Middleware {
	return func(h http.Handler) http.Handler {
		for i := range middlewares {
			h = middlewares[len(middlewares)-1-i](h)
		}
		return h
	}
}

// Identity is a middleware that does nothing. It can be used as a helper when
// building middleware chains.
func Identity() Middleware {
	return func(h http.Handler) http.Handler {
		return h
	}
}
