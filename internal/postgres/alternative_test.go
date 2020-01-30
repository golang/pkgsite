// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/testing/sample"
)

func TestIsAlternativePath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	alternativePaths := []*internal.AlternativeModulePath{
		{Canonical: "rsc.io/quote", Alternative: "github.com/rsc.io/quote"},
		{Canonical: "gocloud.dev", Alternative: "github.com/google/go-cloud"},
	}
	for _, p := range alternativePaths {
		if err := testDB.InsertAlternativeModulePath(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	pkgData := []struct {
		pkgPath, modulePath string
		isAlternative       bool
	}{
		{"rsc.io/quote", "rsc.io/quote", false},
		{"github.com/rsc.io/quote", "github.com/rsc.io/quote", true},
		{"gocloud.dev/blob/s3blob", "gocloud.dev", false},
		{"github.com/google/go-cloud/blob/aws", "github.com/google/go-cloud", true},
		{"github.com/foo/bar", "github.com/foo", false},
	}
	for _, data := range pkgData {
		t.Run(data.pkgPath, func(t *testing.T) {
			got, err := testDB.IsAlternativeModulePath(ctx, data.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if got != data.isAlternative {
				t.Errorf("testDB.IsAlternative(%q) = %t; want = %t", data.pkgPath, got, data.isAlternative)
			}
		})
	}

}

func TestInsertAndDeleteAlternatives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	alternativePaths := []*internal.AlternativeModulePath{
		{Canonical: "rsc.io/quote", Alternative: "github.com/rsc.io/quote"},
		{Canonical: "gocloud.dev", Alternative: "github.com/google/go-cloud"},
	}
	for _, p := range alternativePaths {
		if err := testDB.InsertAlternativeModulePath(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := testDB.getAlternativeModulePath(ctx, p.Alternative)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, p); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}

	pkgData := []struct {
		pkgPath, modulePath string
		isAlternative       bool
	}{
		{"rsc.io/quote", "rsc.io/quote", false},
		{"github.com/rsc.io/quote", "github.com/rsc.io/quote", true},
		{"gocloud.dev/blob/s3blob", "gocloud.dev", false},
		{"github.com/google/go-cloud/blob/aws", "github.com/google/go-cloud", true},
		{"github.com/foo/bar", "github.com/foo", false},
	}
	var versionStates []*internal.IndexVersion
	for _, data := range pkgData {
		v := sample.Version()
		v.ModulePath = data.modulePath
		p := sample.Package()
		p.Path = data.pkgPath
		v.Packages = []*internal.Package{p}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
		now := sample.NowTruncated()
		versionStates = append(versionStates, &internal.IndexVersion{
			Path:      data.modulePath,
			Version:   sample.VersionString,
			Timestamp: now,
		})
	}
	if err := testDB.InsertIndexVersions(ctx, versionStates); err != nil {
		t.Fatal(err)
	}
	gotModuleVersionStates, err := testDB.GetNextVersionsToFetch(ctx, len(pkgData))
	if err != nil {
		t.Fatal(err)
	}

	if len(gotModuleVersionStates) != len(pkgData) {
		t.Fatalf("testDB.GetNextVersionsToFetch(ctx, %d) returned %d version states; want = %d",
			len(pkgData), len(gotModuleVersionStates), len(pkgData))
	}

	for _, data := range pkgData {
		t.Run("GetPackage-"+data.pkgPath, func(t *testing.T) {
			isAlternative, err := testDB.IsAlternativeModulePath(ctx, data.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if isAlternative {
				if err := testDB.DeleteAlternatives(ctx, data.modulePath); err != nil {
					t.Fatal(err)
				}
			}

			got, err := testDB.GetPackage(ctx, data.pkgPath, data.modulePath, internal.LatestVersion)
			if data.isAlternative {
				if !errors.Is(err, derrors.NotFound) {
					t.Errorf("Expected pkg %q to be deleted because it is does not have a canonical path", data.modulePath)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if got.Path != data.pkgPath || got.ModulePath != data.modulePath {
				t.Errorf("testDB.GetPackage(%q, latest) = %q, %q; want = %q, %q)", data.pkgPath, got.Path, got.ModulePath, data.pkgPath, data.modulePath)
			}

			gotModuleVersionState, err := testDB.GetModuleVersionState(ctx, data.modulePath, sample.VersionString)
			if err != nil {
				t.Fatal(err)
			}
			wantCode := derrors.ToHTTPStatus(derrors.AlternativeModule)
			if gotModuleVersionState.Status != nil && *gotModuleVersionState.Status != wantCode {
				t.Fatalf("testDB.GetModuleVersionState(ctx, %q, %q) returned status = %d; want = %d", data.modulePath, sample.VersionString, gotModuleVersionState.Status, wantCode)
			}
		})
	}
}
