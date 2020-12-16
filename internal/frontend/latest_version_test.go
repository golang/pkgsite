// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestLatestMinorVersion(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)
	var persistedModules = []testModule{
		{
			path:            "github.com/mymodule/av1module",
			redistributable: true,
			versions:        []string{"v1.0.0", "v1.0.1"},
			packages: []testPackage{
				{
					suffix:         "bar",
					doc:            sample.DocumentationHTML.String(),
					readmeContents: sample.ReadmeContents,
					readmeFilePath: sample.ReadmeFilePath,
				},
			},
		},
	}
	tt := []struct {
		name             string
		fullPath         string
		modulePath       string
		wantMinorVersion string
		wantErr          error
	}{
		{
			name:             "get latest minor version for a persisted module",
			fullPath:         "github.com/mymodule/av1module",
			modulePath:       internal.UnknownModulePath,
			wantMinorVersion: "v1.0.1",
			wantErr:          nil,
		},
		{
			name:             "module does not exist",
			fullPath:         "github.com/mymodule/doesnotexist",
			modulePath:       internal.UnknownModulePath,
			wantMinorVersion: "",
			wantErr:          fmt.Errorf("error while retriving minor version"),
		},
	}
	ctx := context.Background()
	insertTestModules(ctx, t, persistedModules)
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			v, err := latestMinorVersion(ctx, testDB, tc.fullPath, tc.modulePath)
			if err != nil {
				if tc.wantErr == nil {
					t.Fatalf("got %v, want no error", err)
				}
				return
			}
			if v != tc.wantMinorVersion {
				t.Fatalf("got %q, want %q", tc.wantMinorVersion, v)
			}
		})
	}
}
