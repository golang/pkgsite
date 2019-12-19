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
)

//lint:file-ignore ST1012 prefixing error values with Err would stutter

var (
	// NotFound indicates that a requested entity was not found (HTTP 404).
	NotFound = errors.New("not found")
	// InvalidArgument indicates that the input into the request is invalid in
	// some way (HTTP 400).
	InvalidArgument = errors.New("invalid argument")
	// BadModule indicates a problem with a module.
	BadModule = errors.New("bad module")
	// Excluded indicates that the module is excluded. (See internal/postgres/excluded.go.)
	Excluded = errors.New("excluded")

	// Unknown indicates that the error has unknown semantics.
	Unknown = errors.New("unknown")
)

var httpCodes = []struct {
	err  error
	code int
}{
	{NotFound, http.StatusNotFound},
	{InvalidArgument, http.StatusBadRequest},
	{BadModule, 490}, // since this isn't an HTTP status, pick an unused code
	{Excluded, http.StatusForbidden},
}

// FromHTTPStatus generates an error according to the HTTP semantics for the given
// status code. It uses the given format string and arguments to create the
// error string according to the fmt package.
//
// If HTTP semantics indicate success, it returns nil.
func FromHTTPStatus(code int, format string, args ...interface{}) error {
	if code >= 200 && code < 300 {
		return nil
	}
	var innerErr = Unknown
	for _, e := range httpCodes {
		if e.code == code {
			innerErr = e.err
			break
		}
	}
	return fmt.Errorf(format+": %w", append(args, innerErr)...)
}

// ToHTTPStatus returns an HTTP status code corresponding to err.
func ToHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	for _, e := range httpCodes {
		if errors.Is(err, e.err) {
			return e.code
		}
	}
	return http.StatusInternalServerError
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
		*errp = fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), *errp)
	}
}
