// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetchdatasource

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/pkgsite/internal/version"
)

var (
	defaultTestModules []*proxytest.Module
	localGetters       []fetch.ModuleGetter
)

func TestMain(m *testing.M) {
	dochtml.LoadTemplates(template.TrustedFSFromTrustedSource(template.TrustedSourceFromConstant("../../static")))
	defaultTestModules = proxytest.LoadTestModules("../proxy/testdata")
	var cleanup func()
	localGetters, cleanup = buildLocalGetters()
	defer cleanup()
	licenses.OmitExceptions = true
	os.Exit(m.Run())
}

func buildLocalGetters() ([]fetch.ModuleGetter, func()) {
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

	var (
		dirs    []string
		getters []fetch.ModuleGetter
	)
	ctx := context.Background()
	for _, module := range modules {
		directory, err := testhelper.CreateTestDirectory(module)
		if err != nil {
			log.Fatal(err)
		}
		dirs = append(dirs, directory)
		mg, err := fetch.NewGoPackagesModuleGetter(ctx, directory, "./...")
		if err != nil {
			log.Fatal(err)
		}
		getters = append(getters, mg)
	}
	return getters, func() {
		for _, d := range dirs {
			os.RemoveAll(d)
		}
	}
}

func setup(t *testing.T, testModules []*proxytest.Module, bypassLicenseCheck bool) (context.Context, *FetchDataSource, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)

	var client *proxy.Client
	teardownProxy := func() {}
	if testModules != nil {
		client, teardownProxy = proxytest.SetupTestClient(t, testModules)
	}

	getters := localGetters
	if testModules != nil {
		getters = append(getters, fetch.NewProxyModuleGetter(client, source.NewClientForTesting()))
	}

	return ctx, Options{
			Getters:              getters,
			ProxyClientForLatest: client,
			BypassLicenseCheck:   bypassLicenseCheck,
		}.New(), func() {
			teardownProxy()
			cancel()
		}
}

func TestProxyGetUnitMeta(t *testing.T) {
	ctx, ds, teardown := setup(t, defaultTestModules, false)
	defer teardown()

	singleModInfo := internal.ModuleInfo{
		ModulePath:        "example.com/single",
		Version:           "v1.0.0",
		IsRedistributable: true,
		CommitTime:        proxytest.CommitTime,
		HasGoMod:          true,
	}

	for _, test := range []struct {
		path, modulePath, version string
		want                      *internal.UnitMeta
	}{
		{
			path:       "example.com/single",
			modulePath: "example.com/single",
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/single/pkg",
			modulePath: "example.com/single",
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				Name:              "pkg",
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/single/pkg",
			modulePath: internal.UnknownModulePath,
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				Name:              "pkg",
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/basic",
			modulePath: internal.UnknownModulePath,
			version:    version.Latest,
			want: &internal.UnitMeta{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "example.com/basic",
					Version:           "v1.1.0",
					IsRedistributable: true,
					CommitTime:        proxytest.CommitTime,
					HasGoMod:          true,
				},
				Name:              "basic",
				IsRedistributable: true,
			},
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := ds.GetUnitMeta(ctx, test.path, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}
			test.want.Path = test.path
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreFields(internal.ModuleInfo{}, "SourceInfo")); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBypass(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%t", bypass), func(t *testing.T) {
			// re-create the data source to get around caching
			ctx, ds, teardown := setup(t, defaultTestModules, bypass)
			defer teardown()
			for _, test := range []struct {
				path      string
				wantEmpty bool
			}{
				{"example.com/basic", false},
				{"example.com/nonredist/unk", !bypass},
			} {
				t.Run(test.path, func(t *testing.T) {
					um, err := ds.GetUnitMeta(ctx, test.path, internal.UnknownModulePath, "v1.0.0")
					if err != nil {
						t.Fatal(err)
					}
					got, err := ds.GetUnit(ctx, um, 0, internal.BuildContext{})
					if err != nil {
						t.Fatal(err)
					}

					// Assume internal.Module.RemoveNonRedistributableData is correct; we just
					// need to check one value to confirm that it was called.
					if gotEmpty := (got.Documentation == nil); gotEmpty != test.wantEmpty {
						t.Errorf("got empty %t, want %t", gotEmpty, test.wantEmpty)
					}
				})
			}
		})
	}
}

func TestGetLatestInfo(t *testing.T) {
	testModules := []*proxytest.Module{
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.1.0",
			Files: map[string]string{
				"baz.go": "package bar",
			},
		},
		{
			ModulePath: "foo.com/bar/v2",
			Version:    "v2.0.5",
		},
		{
			ModulePath: "foo.com/bar/v3",
		},
		{
			ModulePath: "bar.com/foo",
			Version:    "v1.1.0",
			Files: map[string]string{
				"baz.go": "package foo",
			},
		},
		{
			ModulePath: "incompatible.com/bar",
			Version:    "v2.1.1+incompatible",
			Files: map[string]string{
				"baz.go": "package bar",
			},
		},
		{
			ModulePath: "incompatible.com/bar/v3",
		},
	}
	ctx, ds, teardown := setup(t, testModules, false)
	defer teardown()
	for _, test := range []struct {
		fullPath        string
		modulePath      string
		wantModulePath  string
		wantPackagePath string
		wantErr         error
	}{
		{
			fullPath:        "foo.com/bar",
			modulePath:      "foo.com/bar",
			wantModulePath:  "foo.com/bar/v3",
			wantPackagePath: "foo.com/bar/v3",
		},
		{
			fullPath:        "bar.com/foo",
			modulePath:      "bar.com/foo",
			wantModulePath:  "bar.com/foo",
			wantPackagePath: "bar.com/foo",
		},
		{
			fullPath:   "boo.com/far",
			modulePath: "boo.com/far",
			wantErr:    derrors.NotFound,
		},
		{
			fullPath:   "foo.com/bar/baz",
			modulePath: "foo.com/bar",
			wantErr:    derrors.NotFound,
		},
		{
			fullPath:        "incompatible.com/bar",
			modulePath:      "incompatible.com/bar",
			wantModulePath:  "incompatible.com/bar/v3",
			wantPackagePath: "incompatible.com/bar/v3",
		},
	} {
		gotLatest, err := ds.GetLatestInfo(ctx, test.fullPath, test.modulePath, nil)
		if err != nil {
			if test.wantErr == nil {
				t.Fatalf("got unexpected error %v", err)
			}
			if !errors.Is(err, test.wantErr) {
				t.Errorf("got err = %v, want Is(%v)", err, test.wantErr)
			}
		}
		if gotLatest.MajorModulePath != test.wantModulePath || gotLatest.MajorUnitPath != test.wantPackagePath {
			t.Errorf("ds.GetLatestMajorVersion(%v, %v) = (%v, %v), want = (%v, %v)",
				test.fullPath, test.modulePath, gotLatest.MajorModulePath, gotLatest.MajorUnitPath, test.wantModulePath, test.wantPackagePath)
		}
	}
}

func TestLocalGetUnitMeta(t *testing.T) {
	ctx, ds, teardown := setup(t, defaultTestModules, true)
	defer teardown()

	sourceInfo := source.FilesInfo("XXX")

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
		{
			// Module is known but path isn't in it.
			path:       "github.com/my/module/unk",
			modulePath: "github.com/my/module",
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
				var gotURL string
				if got.SourceInfo != nil {
					gotURL = got.SourceInfo.RepoURL()
				}
				const wantRegexp = "^/files/.*/github.com/my/module/$"
				matched, err := regexp.MatchString(wantRegexp, gotURL)
				if err != nil {
					t.Fatal(err)
				}
				if !matched {
					t.Errorf("RepoURL: got %q, want match of %q", gotURL, wantRegexp)
				}
				opts := []cmp.Option{
					cmp.AllowUnexported(source.Info{}),
					cmpopts.IgnoreFields(source.Info{}, "repoURL"),
					cmpopts.IgnoreFields(internal.ModuleInfo{}, "CommitTime"), // commit time is volatile, based on file mtimes
				}
				diff := cmp.Diff(test.want, got, opts...)
				if diff != "" {
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
	ctx, ds, teardown := setup(t, nil, true)
	defer teardown()

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
			got, err := ds.GetUnit(ctx, um, 0, internal.BuildContext{})
			if !test.wantLoaded {
				if err == nil {
					t.Fatal("returned not loaded module")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if gotEmpty := (got.Documentation == nil && got.Readme == nil); gotEmpty {
				t.Errorf("gotEmpty = %t", gotEmpty)
			}
			if got.Documentation != nil {
				want := []internal.BuildContext{internal.BuildContextAll}
				if !cmp.Equal(got.BuildContexts, want) {
					t.Errorf("got %v, want %v", got.BuildContexts, want)
				}
			}
		})
	}
}

func TestBuildConstraints(t *testing.T) {
	// The Unit returned by GetUnit should have a single Documentation that
	// matches the BuildContext argument.
	ctx, ds, teardown := setup(t, defaultTestModules, true)
	defer teardown()

	um := &internal.UnitMeta{
		Path: "example.com/build-constraints/cpu",
		ModuleInfo: internal.ModuleInfo{
			ModulePath: "example.com/build-constraints",
			Version:    version.Latest,
		},
	}
	for _, test := range []struct {
		in, want internal.BuildContext
	}{
		{internal.BuildContext{}, internal.BuildContextLinux},
		{internal.BuildContextLinux, internal.BuildContextLinux},
		{internal.BuildContextDarwin, internal.BuildContextDarwin},
		{internal.BuildContext{GOOS: "LiverPaté", GOARCH: "DeTriomphe"}, internal.BuildContext{}},
	} {
		t.Run(test.in.String(), func(t *testing.T) {
			u, err := ds.GetUnit(ctx, um, internal.AllFields, test.in)
			if err != nil {
				t.Fatal(err)
			}
			if test.want == (internal.BuildContext{}) {
				if len(u.Documentation) != 0 {
					t.Error("got docs, want none")
				}
			} else if n := len(u.Documentation); n != 1 {
				t.Errorf("got %d docs, want 1", n)
			} else if got := u.Documentation[0].BuildContext(); got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestCache(t *testing.T) {
	ds := Options{}.New()
	m1 := &internal.Module{}
	ds.cachePut(nil, "m1", fetch.LocalVersion, m1, nil)
	ds.cachePut(nil, "m2", "v1.0.0", nil, derrors.NotFound)

	for _, test := range []struct {
		path, version string
		wantm         *internal.Module
		wante         error
	}{
		{"m1", fetch.LocalVersion, m1, nil},
		{"m1", "v1.2.3", m1, nil}, // find m1 under LocalVersion
		{"m2", "v1.0.0", nil, derrors.NotFound},
		{"m3", "v1.0.0", nil, nil},
	} {
		_, gotm, gote := ds.cacheGet(test.path, test.version)
		if gotm != test.wantm || gote != test.wante {
			t.Errorf("%s@%s: got (%v, %v), want (%v, %v)", test.path, test.version, gotm, gote, test.wantm, test.wante)
		}
	}
}
