// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package derrors

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"golang.org/x/xerrors"
)

func TestFromHTTPStatus(t *testing.T) {
	tests := []struct {
		label  string
		status int
		want   error
	}{
		{
			label:  "OK translates to nil error",
			status: 200,
		},
		{
			label:  "400 translates to invalid argument",
			status: 400,
			want:   InvalidArgument,
		},
		// Testing other specific HTTP status codes is intentionally omitted to
		// avoid writing a change detector.
	}

	for _, test := range tests {
		test := test
		t.Run(test.label, func(t *testing.T) {
			err := FromHTTPStatus(test.status, "error")
			if !xerrors.Is(err, test.want) {
				t.Errorf("FromHTTPStatus(%d, ...) = %v, want %v", test.status, err, test.want)
			}
		})
	}
}

func TestToHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		in   error
		want int
	}{
		{nil, http.StatusOK},
		{InvalidArgument, http.StatusBadRequest},
		{NotFound, http.StatusNotFound},
		{BadModule, 490},
		{Gone, http.StatusGone},
		{Unknown, http.StatusInternalServerError},
		{xerrors.Errorf("wrapping: %w", NotFound), http.StatusNotFound},
		{io.ErrUnexpectedEOF, http.StatusInternalServerError},
	} {
		got := ToHTTPStatus(tc.in)
		if got != tc.want {
			t.Errorf("ToHTTPStatus(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestAdd(t *testing.T) {
	var err error
	Add(&err, "whatever")
	if err != nil {
		t.Errorf("got %v, want nil", err)
	}

	err = errors.New("bad stuff")
	Add(&err, "Frob(%d)", 3)
	want := "Frob(3): bad stuff"
	if got := err.Error(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := xerrors.Unwrap(err); got != nil {
		t.Errorf("Unwrap: got %v, want nil", got)
	}
}

func TestWrap(t *testing.T) {
	var err error
	Wrap(&err, "whatever")
	if err != nil {
		t.Errorf("got %v, want nil", err)
	}

	orig := errors.New("bad stuff")
	err = orig
	Wrap(&err, "Frob(%d)", 3)
	want := "Frob(3): bad stuff"
	if got := err.Error(); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := xerrors.Unwrap(err); got != orig {
		t.Errorf("Unwrap: got %#v, want %#v", got, orig)
	}
}
