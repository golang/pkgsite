// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tri

import (
	"bytes"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		val  Bool
		want string
	}{
		{Yes, "Yes"},
		{No, "No"},
		{Maybe, "Maybe"},
		{Bool(3), "tri.Bool(3)"},
	}
	for _, tc := range tests {
		if got := tc.val.String(); got != tc.want {
			t.Errorf("%d.String() = %q, want %q", tc.val, got, tc.want)
		}
	}
}

func TestFromBool(t *testing.T) {
	tests := []struct {
		val  bool
		want Bool
	}{
		{true, Yes},
		{false, No},
	}
	for _, tc := range tests {
		if got := FromBool(tc.val); got != tc.want {
			t.Errorf("FromBool(%t) = %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestAnd(t *testing.T) {
	tests := []struct {
		t1, t2 Bool
		want   Bool
	}{
		{Yes, Yes, Yes},
		{Yes, No, No},
		{Yes, Maybe, Maybe},
		{No, Yes, No},
		{No, No, No},
		{No, Maybe, No},
		{Maybe, Yes, Maybe},
		{Maybe, No, No},
		{Maybe, Maybe, Maybe},
	}
	for _, tc := range tests {
		if got := tc.t1.And(tc.t2); got != tc.want {
			t.Errorf("%v.And(%v) = %v, want %v", tc.t1, tc.t2, got, tc.want)
		}
	}
}

func TestOr(t *testing.T) {
	tests := []struct {
		t1, t2 Bool
		want   Bool
	}{
		{Yes, Yes, Yes},
		{Yes, No, Yes},
		{Yes, Maybe, Yes},
		{No, Yes, Yes},
		{No, No, No},
		{No, Maybe, Maybe},
		{Maybe, Yes, Yes},
		{Maybe, No, Maybe},
		{Maybe, Maybe, Maybe},
	}
	for _, tc := range tests {
		if got := tc.t1.Or(tc.t2); got != tc.want {
			t.Errorf("%v.Or(%v) = %v, want %v", tc.t1, tc.t2, got, tc.want)
		}
	}
}

func TestNot(t *testing.T) {
	tests := []struct {
		val  Bool
		want Bool
	}{
		{Yes, No},
		{No, Yes},
		{Maybe, Maybe},
	}
	for _, tc := range tests {
		if got := tc.val.Not(); got != tc.want {
			t.Errorf("%v.Not() = %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		val  Bool
		want []byte
	}{
		{Yes, yesBytes},
		{No, noBytes},
		{Maybe, maybeBytes},
	}
	for _, tc := range tests {
		got, err := tc.val.MarshalJSON()
		if err != nil {
			t.Errorf("%v.MarshalJSON() unexpected error: %v", tc.val, err)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%v.MarshalJSON() = %q, want %q", tc.val, got, tc.want)
		}
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input []byte
		want  Bool
	}{
		{yesBytes, Yes},
		{noBytes, No},
		{maybeBytes, Maybe},
	}
	for _, tc := range tests {
		var got Bool
		err := got.UnmarshalJSON(tc.input)
		if err != nil {
			t.Errorf("UnmarshalJSON(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("UnmarshalJSON(%q) to %v, want %v", tc.input, got, tc.want)
		}
	}
}
