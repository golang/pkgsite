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

	// AlternativeModule indicates that the path of the module zip file differs
	// from the path specified in the go.mod file.
	AlternativeModule = errors.New("alternative module")

	// Unknown indicates that the error has unknown semantics.
	Unknown = errors.New("unknown")

	// BuildContextNotSupported indicates that the build context for the
	// package is not supported.
	BuildContextNotSupported = errors.New("build context not supported")
	// MaxImportsLimitExceeded indicates that the package has too many
	// imports.
	MaxImportsLimitExceeded = errors.New("max imports limit exceeded")
	// MaxFileSizeLimitExceeded indicates that the package contains a file
	// that exceeds fetch.MaxFileSize.
	MaxFileSizeLimitExceeded = errors.New("max file size limit exceeded")
	// DocumentationHTMLTooLarge indicates that the rendered documentation
	// HTML size exceeded the specified limit for dochtml.RenderOptions.
	DocumentationHTMLTooLarge = errors.New("documentation HTML is too large")
	// BadPackage represents an error loading a package because its
	// contents do not make up a valid package. This can happen, for
	// example, if the .go files fail to parse or declare different package
	// names.
	// TODO(b/133187024): break this error up more granularly
	BadPackage = errors.New("bad package")
)

var httpCodes = []struct {
	err  error
	code int
}{
	{NotFound, http.StatusNotFound},
	{InvalidArgument, http.StatusBadRequest},
	{Excluded, http.StatusForbidden},
	// Since the following aren't HTTP statuses, pick unused codes.
	{BadModule, 490},
	{AlternativeModule, 491},
	{BuildContextNotSupported, 600},
	{MaxImportsLimitExceeded, 601},
	{MaxFileSizeLimitExceeded, 602},
	{DocumentationHTMLTooLarge, 603},
	{BadPackage, 604},
}

// FromHTTPStatus generates an error according to the HTTP semantics for the given
// status code. It uses the given format string and arguments to create the
// error string according to the fmt package. If format is the empty string,
// then the error corresponding to the code is returned unwrapped.
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
	if format == "" {
		return innerErr
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
