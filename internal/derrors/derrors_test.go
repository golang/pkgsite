// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package derrors

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
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
			err := FromStatus(test.status, "error")
			if !errors.Is(err, test.want) {
				t.Errorf("FromHTTPStatus(%d, ...) = %v, want %v", test.status, err, test.want)
			}
		})
	}
}

func TestToHTTPStatus(t *testing.T) {
	for _, test := range []struct {
		in   error
		want int
	}{
		{nil, http.StatusOK},
		{InvalidArgument, http.StatusBadRequest},
		{NotFound, http.StatusNotFound},
		{BadModule, 490},
		{AlternativeModule, 491},
		{Unknown, http.StatusInternalServerError},
		{fmt.Errorf("wrapping: %w", NotFound), http.StatusNotFound},
		{io.ErrUnexpectedEOF, http.StatusInternalServerError},
	} {
		got := ToStatus(test.in)
		if got != test.want {
			t.Errorf("ToHTTPStatus(%v) = %d, want %d", test.in, got, test.want)
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
	if got := errors.Unwrap(err); got != nil {
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
	if got := errors.Unwrap(err); got != orig {
		t.Errorf("Unwrap: got %#v, want %#v", got, orig)
	}
}

func TestWrapStack(t *testing.T) {
	var err error = io.ErrShortWrite
	WrapStack(&err, "while frobbing")
	if !errors.Is(err, io.ErrShortWrite) {
		t.Error("is not io.ErrShortWrite")
	}
	var se *StackError
	if !errors.As(err, &se) {
		t.Fatal("not as StackError")
	}
	if !strings.Contains(string(se.Stack), "WrapStack") {
		t.Fatal("bad stack trace")
	}
}
