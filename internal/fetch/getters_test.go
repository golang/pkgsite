// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"errors"
	"testing"

	"golang.org/x/pkgsite/internal/derrors"
)

func TestDirectoryModuleGetterEmpty(t *testing.T) {
	g, err := NewDirectoryModuleGetter("", "testdata/has_go_mod")
	if err != nil {
		t.Fatal(err)
	}
	if want := "example.com/testmod"; g.modulePath != want {
		t.Errorf("got %q, want %q", g.modulePath, want)
	}

	_, err = NewDirectoryModuleGetter("", "testdata/no_go_mod")
	if !errors.Is(err, derrors.BadModule) {
		t.Errorf("got %v, want BadModule", err)
	}
}
