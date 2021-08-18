// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/source"
)

func TestLocalEmptyModulePath(t *testing.T) {
	// Test local fetching when the module path is empty (corresponding to the
	// main module of a directory). Other cases are tested in TestFetchModule.
	ctx := context.Background()
	got := FetchLocalModule(ctx, "", "testdata/has_go_mod", source.NewClientForTesting())
	if got.Error != nil {
		t.Fatal(got.Error)
	}
	if want := "testmod"; got.ModulePath != want {
		t.Errorf("got %q, want %q", got.ModulePath, want)
	}

	got = FetchLocalModule(ctx, "", "testdata/no_go_mod", source.NewClientForTesting())
	if !errors.Is(got.Error, derrors.BadModule) {
		t.Errorf("got %v, want BadModule", got.Error)
	}
}
