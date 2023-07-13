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
			name:             "package",
			fullPath:         "github.com/mymodule/av1module/bar",
			modulePath:       "github.com/mymodule/av1module",
			wantMinorVersion: "v1.0.1",
		},
		{
			name:             "module",
			fullPath:         "github.com/mymodule/av1module",
			modulePath:       "github.com/mymodule/av1module",
			wantMinorVersion: "v1.0.1",
		},
		{
			name:       "module does not exist",
			fullPath:   "github.com/mymodule/doesnotexist",
			modulePath: internal.UnknownModulePath,
			wantErr:    fmt.Errorf("error while retrieving minor version"),
		},
	}
	ctx := context.Background()
	insertTestModules(ctx, t, persistedModules)
	svr := &Server{getDataSource: func(context.Context) internal.DataSource { return testDB }}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := svr.GetLatestInfo(ctx, tc.fullPath, tc.modulePath, nil)
			if got.MinorVersion != tc.wantMinorVersion {
				t.Fatalf("got %q, want %q", tc.wantMinorVersion, got.MinorVersion)
			}
		})
	}
}
