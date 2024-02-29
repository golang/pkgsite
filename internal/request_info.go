// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"net/http"
	"time"
)

// RequestInfo is information about an HTTP request.
type RequestInfo struct {
	Request    *http.Request
	TrimmedURL string      // URL with the scheme and host removed
	TraceID    string      // extracted from request header
	Start      time.Time   // when the request began
	Cancel     func(error) // function that cancels the request's context
}

func NewRequestInfo(r *http.Request) *RequestInfo {
	turl := *r.URL
	turl.Scheme = ""
	turl.Host = ""
	turl.User = nil
	return &RequestInfo{
		Request:    r,
		TrimmedURL: turl.String(),
		TraceID:    r.Header.Get("X-Cloud-Trace-Context"),
		Start:      time.Now(),
	}
}

// requestInfoKey is the type of the context key for RequestInfos.
type requestInfoKey struct{}

// NewContextWithRequestInfo creates a new context from ctx that adds the trace ID.
func NewContextWithRequestInfo(ctx context.Context, ri *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, ri)
}

// RequestInfoFromContext retrieves the trace ID from the context, or a zero one
// if it isn't there.
func RequestInfoFromContext(ctx context.Context) *RequestInfo {
	ri, _ := ctx.Value(requestInfoKey{}).(*RequestInfo)
	if ri == nil {
		return &RequestInfo{}
	}
	return ri
}
