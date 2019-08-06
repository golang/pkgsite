// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package derrors

import (
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
