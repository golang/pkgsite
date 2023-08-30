// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package trace provides a wrapper around third party tracing
// libraries
package trace

import "context"

// Span is an interface for a type that ends a span.
type Span interface {
	End()
}

var startSpan func(context.Context, string) (context.Context, Span)

// SetTraceFunction sets StartSpan to call the given function to start
// a trace span.
func SetTraceFunction(f func(context.Context, string) (context.Context, Span)) {
	startSpan = f
}

// If SetTraceFunction has been called, StartSpan uses its given
// function to start a span. Otherwise it does nothing.
func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	if startSpan != nil {
		return startSpan(ctx, name)
	}
	return ctx, trivialSpan{}
}

type trivialSpan struct{}

func (trivialSpan) End() {}
