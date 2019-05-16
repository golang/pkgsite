// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package derrors

import (
	"errors"
	"net/http"
	"testing"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		label                          string
		err                            error
		isNotFound, isInvalidArguments bool
		wantType                       ErrorType
	}{
		{
			label:      "identifies not found errors",
			err:        NotFound("couldn't find it"),
			isNotFound: true,
			wantType:   NotFoundType,
		}, {
			label:              "identifies invalid argument errors",
			err:                InvalidArgument("bad arguments"),
			isInvalidArguments: true,
			wantType:           InvalidArgumentType,
		}, {
			label:    "doesn't identify an unknown error",
			err:      errors.New("bad"),
			wantType: UncategorizedErrorType,
		}, {
			label:    "doesn't identify a nil error",
			wantType: NilErrorType,
		}, {
			label:              "wraps a known error",
			err:                Wrap(InvalidArgument("bad arguments"), "error validating %s", "abc123"),
			isInvalidArguments: true,
			wantType:           InvalidArgumentType,
		}, {
			label:      "interprets HTTP 404 as Not Found",
			err:        StatusError(http.StatusNotFound, "%s was not found", "foo"),
			isNotFound: true,
			wantType:   NotFoundType,
		}, {
			label:    "interprets HTTP 500 as Uncategorized",
			err:      StatusError(http.StatusInternalServerError, "bad"),
			wantType: UncategorizedErrorType,
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			if got := IsNotFound(test.err); got != test.isNotFound {
				t.Errorf("IsNotFound(%v) = %t, want %t", test.err, got, test.isNotFound)
			}
			if got := IsInvalidArgument(test.err); got != test.isInvalidArguments {
				t.Errorf("IsInvalidArguments(%v) = %t, want %t", test.err, got, test.isInvalidArguments)
			}
			if got := Type(test.err); got != test.wantType {
				t.Errorf("Type(%v) = %v, want %v", test.err, got, test.wantType)
			}
		})
	}
}
