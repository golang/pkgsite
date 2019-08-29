// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package derrors defines internal error values to categorize the different
// types error semantics we support.
package derrors

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"
)

//lint:file-ignore ST1012 prefixing error values with Err would stutter

var (
	// NotFound indicates that a requested entity was not found (HTTP 404)
	NotFound = errors.New("not found")
	// NotAcceptable indicates that the requested entity is not valid in this
	// context (somewhat related to HTTP 406, though meanings are overloaded).
	NotAcceptable = errors.New("not acceptable")
	// InvalidArgument indicates that the input into the request is invalid in
	// some way (HTTP 400)
	InvalidArgument = errors.New("invalid argument")
	// Gone indicates that the requested entity was not found, and that this is
	// likely to be a permanent condition (HTTP 410).
	Gone = errors.New("gone")
	// Unknown indicates that the error has unknown semantics.
	Unknown = errors.New("unknown")
)

// FromHTTPStatus generates an error according to the HTTP semantics for the given
// status code. It uses the given format string and arguments to create the
// error string according to the fmt package.
//
// If HTTP semantics indicate success, it returns nil.
func FromHTTPStatus(code int, format string, args ...interface{}) error {
	if code >= 200 && code < 300 {
		return nil
	}
	var innerErr error
	switch code {
	case http.StatusBadRequest:
		innerErr = InvalidArgument
	case http.StatusNotAcceptable:
		innerErr = NotAcceptable
	case http.StatusNotFound:
		innerErr = NotFound
	case http.StatusGone:
		innerErr = Gone
	default:
		innerErr = Unknown
	}
	return xerrors.Errorf(format+": %w", append(args, innerErr)...)
}

// ToHTTPStatus returns an HTTP status code corresponding to err.
func ToHTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case xerrors.Is(err, NotFound):
		return http.StatusNotFound
	case xerrors.Is(err, NotAcceptable):
		return http.StatusNotAcceptable
	case xerrors.Is(err, InvalidArgument):
		return http.StatusBadRequest
	case xerrors.Is(err, Gone):
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

// Add adds context to the error.
// The result cannot be unwrapped to recover the original error.
// It does nothing when *errp == nil.
//
// Example:
//
//	defer derrors.Add(&err, "copy(%s, %s)", src, dst)
//
// See Wrap for an equivalent function that allows
// the result to be unwrapped.
func Add(errp *error, format string, args ...interface{}) {
	if *errp != nil {
		*errp = fmt.Errorf("%s: %v", fmt.Sprintf(format, args...), *errp)
	}
}

// Wrap adds context to the error and allows
// unwrapping the result to recover the original error.
//
// Example:
//
//	defer derrors.Wrap(&err, "copy(%s, %s)", src, dst)
//
// See Add for an equivalent function that does not allow
// the result to be unwrapped.
func Wrap(errp *error, format string, args ...interface{}) {
	if *errp != nil {
		*errp = xerrors.Errorf("%s: %w", fmt.Sprintf(format, args...), *errp)
	}
}
