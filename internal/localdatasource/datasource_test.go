// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localdatasource

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var (
	ctx        context.Context
	cancel     func()
	datasource *DataSource
)

func setup(t *testing.T) (context.Context, func(), *DataSource, error) {
	t.Helper()

	// Setup only once.
	if datasource != nil {
		return ctx, cancel, datasource, nil
	}

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

	dochtml.LoadTemplates(template.TrustedSourceFromConstant("../../content/static/html/doc"))
	datasource = New()
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	for _, module := range modules {
		directory, err := testhelper.CreateTestDirectory(module)
		if err != nil {
			return ctx, func() { cancel() }, nil, err
		}
		defer os.RemoveAll(directory)

		err = datasource.Load(ctx, directory)
		if err != nil {
			return ctx, func() { cancel() }, nil, err
		}
	}

	return ctx, func() { cancel() }, datasource, nil
}

func TestGetUnitMeta(t *testing.T) {
	ctx, cancel, ds, err := setup(t)
	if err != nil {
		t.Fatalf("setup failed: %s", err.Error())
	}
	defer cancel()

	for _, test := range []struct {
		path, modulePath string
		want             *internal.UnitMeta
		wantErr          error
	}{
		{
			path:       "github.com/my/module",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path:              "github.com/my/module",
				ModulePath:        "github.com/my/module",
				IsRedistributable: true,
				Version:           fetch.LocalVersion,
				CommitTime:        fetch.LocalCommitTime,
			},
		},
		{
			path:       "github.com/my/module/bar",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path:              "github.com/my/module/bar",
				Name:              "bar",
				ModulePath:        "github.com/my/module",
				IsRedistributable: true,
				Version:           fetch.LocalVersion,
				CommitTime:        fetch.LocalCommitTime,
			},
		},
		{
			path:       "github.com/my/module/foo",
			modulePath: "github.com/my/module",
			want: &internal.UnitMeta{
				Path:              "github.com/my/module/foo",
				Name:              "foo",
				ModulePath:        "github.com/my/module",
				IsRedistributable: true,
				Version:           fetch.LocalVersion,
				CommitTime:        fetch.LocalCommitTime,
			},
		},
		{
			path:       "github.com/my/module/bar",
			modulePath: internal.UnknownModulePath,
			want: &internal.UnitMeta{
				Path:              "github.com/my/module/bar",
				Name:              "bar",
				ModulePath:        "github.com/my/module",
				IsRedistributable: true,
				Version:           fetch.LocalVersion,
				CommitTime:        fetch.LocalCommitTime,
			},
		},
		{
			path:       "github.com/not/loaded",
			modulePath: internal.UnknownModulePath,
			wantErr:    derrors.NotFound,
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := ds.GetUnitMeta(ctx, test.path, test.modulePath, fetch.LocalVersion)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Errorf("GetUnitMeta(%q, %q): %v; wantErr = %v)", test.path, test.modulePath, err, test.wantErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)

				}
			}
		})
	}
}

func TestGetUnit(t *testing.T) {
	// This is a simple test to verify that data is fetched correctly. The
	// return value of FetchResult is tested in internal/fetch so no need
	// to repeat it.
	ctx, cancel, ds, err := setup(t)
	if err != nil {
		t.Fatalf("setup failed: %s", err.Error())
	}
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
				ModulePath: test.modulePath,
			}
			got, err := ds.GetUnit(ctx, um, 0)
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
