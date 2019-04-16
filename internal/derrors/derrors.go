// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package derrors implements some common error semantics to be used by other
// internal packages.
package derrors

// errorMessage is type that can be embedded to implement the error interface.
type errorMessage string

func (m errorMessage) Error() string {
	return string(m)
}

type notFound struct {
	errorMessage
}

// NotFound creates a new error message that indicates the requested entity is
// not found.
func NotFound(msg string) error {
	return notFound{
		errorMessage: errorMessage(msg),
	}
}

// IsNotFound reports whether err is a NotFound error.
func IsNotFound(err error) bool {
	_, ok := err.(notFound)
	return ok
}

type invalidArguments struct {
	errorMessage
}

// InvalidArguments creates a new error that indicates the given arguments are
// invalid.
func InvalidArguments(msg string) error {
	return invalidArguments{
		errorMessage: errorMessage(msg),
	}
}

// IsInvalidArguments reports whether err is an InvalidArguments error.
func IsInvalidArguments(err error) bool {
	_, ok := err.(invalidArguments)
	return ok
}
