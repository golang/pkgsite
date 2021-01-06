// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestPreviousFetchStatusAndResponse(t *testing.T) {
	ctx := context.Background()

	for _, mod := range []struct {
		path      string
		goModPath string
		status    int
	}{
		{"mvdan.cc/sh", "", 200},
		{"mvdan.cc", "", 404},
		{"400.mod/foo/bar", "", 400},
		{"400.mod/foo", "", 404},
		{"400.mod", "", 404},
		{"github.com/alternative/ok", "github.com/vanity", 491},
		{"github.com/alternative/ok/path", "", 404},
		{"github.com/alternative/bad", "vanity", 491},
		{"bad.mod/foo/bar", "", 490},
		{"bad.mod/foo", "", 404},
		{"bad.mod", "", 490},
		{"500.mod/foo", "", 404},
		{"500.mod", "", 500},
		{"reprocess.mod/foo", "", 520},
	} {
		goModPath := mod.goModPath
		if goModPath == "" {
			goModPath = mod.path
		}
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       mod.path,
			RequestedVersion: internal.LatestVersion,
			ResolvedVersion:  sample.VersionString,
			Status:           mod.status,
			GoModPath:        goModPath,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"bad request at path root", "400.mod/foo/bar", 404},
		{"bad request at mod but 404 at path", "400.mod/foo", 404},
		{"alternative mod", "github.com/alternative/ok", 491},
		{"alternative mod package path", "github.com/alternative/ok/path", 491},
		{"alternative mod bad module path", "github.com/alternative/bad", 404},
		{"bad module at path", "bad.mod/foo/bar", 404},
		{"bad module at mod but 404 at path", "bad.mod/foo", 404},
		{"500", "500.mod/foo", 500},
		{"mod to reprocess", "reprocess.mod/foo", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			fr, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.LatestVersion)
			if err != nil {
				t.Fatal(err)
			}
			if fr.status != test.status {
				t.Errorf("got %v; want %v", fr.status, test.status)
			}
		})
	}

	for _, test := range []struct {
		name, path string
	}{
		{"path never fetched", "github.com/nonexistent"},
		{"path never fetched, but top level mod fetched", "mvdan.cc/sh/v3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := previousFetchStatusAndResponse(ctx, testDB, test.path, internal.LatestVersion)
			if !errors.Is(err, derrors.NotFound) {
				t.Errorf("got %v; want %v", err, derrors.NotFound)
			}
		})
	}
}
