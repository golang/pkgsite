// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var datasource *LocalDataSource

func setupLocal() func() {
	modules := []map[string]string{
		{
			"go.mod":        "module github.com/my/module\n\ngo 1.12",
			"LICENSE":       testhelper.BSD0License,
			"README.md":     "README FILE FOR TESTING.",
			"bar/COPYING":   testhelper.MITLicense,
			"bar/README.md": "Another README FILE FOR TESTING.",
			"bar/bar.go": `
			// package bar
			package bar

			// Bar returns the string "bar".
			func Bar() string {
				return "bar"
			}`,
			"foo/LICENSE.md": testhelper.MITLicense,
			"foo/foo.go": `
			// package foo
			package foo

			import (
				"fmt"

				"github.com/my/module/bar"
			)

			// FooBar returns the string "foo bar".
			func FooBar() string {
				return fmt.Sprintf("foo %s", bar.Bar())
			}`,
		},
		{
			"go.mod":  "module github.com/no/license\n\ngo 1.12",
			"LICENSE": "unknown",
			"bar/bar.go": `
			// package bar
			package bar

			// Bar returns the string "bar".
			func Bar() string {
				return "bar"
			}`,
		},
	}

	datasource = NewLocal(source.NewClientForTesting())
	var dirs []string
	for _, module := range modules {
		directory, err := testhelper.CreateTestDirectory(module)
		if err != nil {
			log.Fatal(err)
		}
		dirs = append(dirs, directory)
		mg, err := fetch.NewDirectoryModuleGetter("", directory)
		if err != nil {
			log.Fatal(err)
		}
		datasource.AddModuleGetter(mg)
	}
	return func() {
		for _, d := range dirs {
			os.RemoveAll(d)
		}
	}
}

func TestLocalGetUnitMeta(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sourceInfo := source.NewGitHubInfo("https://github.com/my/module", "", "v0.0.0")
	for _, test := range []struct {
		path, modulePath string
		want             *internal.UnitMeta
		wantErr          error
	}{
		{
			path:       "github.com/my/module",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path: "github.com/my/module",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "github.com/my/module",
					Version:           fetch.LocalVersion,
					CommitTime:        fetch.LocalCommitTime,
					IsRedistributable: true,
					HasGoMod:          true,
					SourceInfo:        sourceInfo,
				},
				IsRedistributable: true,
			},
		},
		{
			path:       "github.com/my/module/bar",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path: "github.com/my/module/bar",
				Name: "bar",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "github.com/my/module",
					Version:           fetch.LocalVersion,
					CommitTime:        fetch.LocalCommitTime,
					IsRedistributable: true,
					HasGoMod:          true,
					SourceInfo:        sourceInfo,
				},
				IsRedistributable: true,
			},
		},
		{
			path:       "github.com/my/module/foo",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path: "github.com/my/module/foo",
				Name: "foo",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "github.com/my/module",
					IsRedistributable: true,
					Version:           fetch.LocalVersion,
					CommitTime:        fetch.LocalCommitTime,
					HasGoMod:          true,
					SourceInfo:        sourceInfo,
				},
				IsRedistributable: true,
			},
		},
		{
			path:       "github.com/my/module/bar",
			modulePath: internal.UnknownModulePath,
			want: &internal.UnitMeta{
				Path:              "github.com/my/module/bar",
				Name:              "bar",
				IsRedistributable: true,
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "github.com/my/module",
					Version:           fetch.LocalVersion,
					CommitTime:        fetch.LocalCommitTime,
					IsRedistributable: true,
					HasGoMod:          true,
					SourceInfo:        sourceInfo,
				},
			},
		},
		{
			path:       "github.com/not/loaded",
			modulePath: internal.UnknownModulePath,
			wantErr:    derrors.NotFound,
		},
		{
			path:       "net/http",
			modulePath: stdlib.ModulePath,
			wantErr:    derrors.InvalidArgument,
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := datasource.GetUnitMeta(ctx, test.path, test.modulePath, fetch.LocalVersion)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("GetUnitMeta(%q, %q): %v; wantErr = %v)", test.path, test.modulePath, err, test.wantErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(source.Info{})); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)

				}
			}
		})
	}
}

func TestLocalGetUnit(t *testing.T) {
	// This is a simple test to verify that data is fetched correctly. The
	// return value of FetchResult is tested in internal/fetch so no need
	// to repeat it.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, test := range []struct {
		path, modulePath string
		wantLoaded       bool
	}{
		{
			path:       "github.com/my/module",
			modulePath: "github.com/my/module",
			wantLoaded: true,
		},
		{
			path:       "github.com/my/module/foo",
			modulePath: "github.com/my/module",
			wantLoaded: true,
		},
		{
			path:       "github.com/no/license/bar",
			modulePath: "github.com/no/license",
			wantLoaded: true,
		},
		{
			path:       "github.com/not/loaded",
			modulePath: internal.UnknownModulePath,
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			um := &internal.UnitMeta{
				Path:       test.path,
				ModuleInfo: internal.ModuleInfo{ModulePath: test.modulePath},
			}
			got, err := datasource.GetUnit(ctx, um, 0, internal.BuildContext{})
			if !test.wantLoaded {
				if err == nil {
					t.Fatalf("returned not loaded module %q", test.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("failed for %q: %q", test.path, err.Error())
			}

			if gotEmpty := (got.Documentation == nil && got.Readme == nil); gotEmpty {
				t.Errorf("%q: gotEmpty = %t", test.path, gotEmpty)
			}
		})
	}
}
