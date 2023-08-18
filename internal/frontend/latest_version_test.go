// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestLatestMinorVersion(t *testing.T) {
	fds := fakedatasource.New()
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
			wantErr:    fmt.Errorf("error while retriving minor version"),
		},
	}
	ctx := context.Background()
	insertTestModules(ctx, t, fds, persistedModules)
	svr := &Server{getDataSource: func(context.Context) internal.DataSource { return fds }}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := svr.GetLatestInfo(ctx, tc.fullPath, tc.modulePath, nil)
			if got.MinorVersion != tc.wantMinorVersion {
				t.Fatalf("got %q, want %q", tc.wantMinorVersion, got.MinorVersion)
			}
		})
	}
}

func insertTestModules(ctx context.Context, t *testing.T, fds *fakedatasource.FakeDataSource, mods []testModule) {
	for _, mod := range mods {
		var (
			suffixes []string
			pkgs     = make(map[string]testPackage)
		)
		for _, pkg := range mod.packages {
			suffixes = append(suffixes, pkg.suffix)
			pkgs[pkg.suffix] = pkg
		}
		for _, ver := range mod.versions {
			m := sample.Module(mod.path, ver, suffixes...)
			m.SourceInfo = source.NewGitHubInfo(sample.RepositoryURL, "", ver)
			m.IsRedistributable = mod.redistributable
			if !m.IsRedistributable {
				m.Licenses = nil
			}
			for _, u := range m.Units {
				if pkg, ok := pkgs[internal.Suffix(u.Path, m.ModulePath)]; ok {
					if pkg.name != "" {
						u.Name = pkg.name
					}
					if pkg.readmeContents != "" {
						u.Readme = &internal.Readme{
							Contents: pkg.readmeContents,
							Filepath: pkg.readmeFilePath,
						}
					}
					if pkg.docs != nil {
						u.Documentation = pkg.docs
					}
				}
				if !mod.redistributable {
					u.IsRedistributable = false
					u.Licenses = nil
					u.Documentation = nil
					u.Readme = nil
				}
			}
			fds.MustInsertModule(ctx, m)
		}
	}
}
