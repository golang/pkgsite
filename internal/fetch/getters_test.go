// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testenv"
	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/pkgsite/internal/version"
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

const multiModule = `
-- go.work --
go 1.21

use (
	./foo
	./bar
)

-- foo/go.mod --
module foo.com/foo

go 1.21
-- foo/foolog/f.go --
package foolog

const Log = 1
-- bar/go.mod --
module bar.com/bar

go 1.20
-- bar/barlog/b.go --
package barlog

const Log = 1
`

func TestGoPackagesModuleGetter(t *testing.T) {
	testenv.MustHaveExecPath(t, "go")

	modulePaths := map[string]string{ // dir -> module path
		"foo": "foo.com/foo",
		"bar": "bar.com/bar",
	}

	tests := []struct {
		name string
		dir  string
	}{
		{"work dir", "."},
		{"module dir", "foo"},
		{"nested package dir", "foo/foolog"},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir, files := testhelper.WriteTxtarToTempDir(t, multiModule)
			dir := filepath.Join(tempDir, test.dir)

			g, err := NewGoPackagesModuleGetter(ctx, dir, "all")
			if err != nil {
				t.Fatal(err)
			}

			for moduleDir, modulePath := range modulePaths {
				t.Run("info", func(t *testing.T) {
					got, err := g.Info(ctx, modulePath, "")
					if err != nil {
						t.Fatal(err)
					}
					if got, want := got.Version, LocalVersion; got != want {
						t.Errorf("Info(%s): got version %s, want %s", modulePath, got, want)
					}
				})

				mod := files[moduleDir+"/go.mod"]
				t.Run("mod", func(t *testing.T) {
					got, err := g.Mod(ctx, modulePath, "")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(mod, string(got)); diff != "" {
						t.Errorf("Mod(%q) mismatch [-want +got]:\n%s", modulePath, diff)
					}
				})

				t.Run("contentdir", func(t *testing.T) {
					fsys, err := g.ContentDir(ctx, modulePath, "")
					if err != nil {
						t.Fatal(err)
					}
					// Just check that the go.mod file is there and has the right contents.
					got, err := fs.ReadFile(fsys, "go.mod")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(mod, string(got)); diff != "" {
						t.Errorf("fs.ReadFile(ContentDir(%q), %q) mismatch [-want +got]:\n%s", modulePath, "go.mod", diff)
					}
				})

				t.Run("search", func(t *testing.T) {
					tests := []struct {
						query string
						want  []string
					}{
						{"log", []string{"barlog", "foolog"}},
						{"barlog", []string{"barlog"}},
						{"xxxxxx", nil},
					}

					for _, test := range tests {
						results, err := g.Search(ctx, test.query, 10)
						if err != nil {
							t.Fatal(err)
						}
						var got []string
						for _, r := range results {
							got = append(got, r.Name)
						}
						if diff := cmp.Diff(test.want, got); diff != "" {
							t.Errorf("Search(%s) mismatch [-want +got]:\n%s", test.query, diff)
						}
					}
				})
			}
		})
	}
}

func TestGoPackagesModuleGetter_Invalidation(t *testing.T) {
	testenv.MustHaveExecPath(t, "go")

	ctx := context.Background()

	tempDir, _ := testhelper.WriteTxtarToTempDir(t, multiModule)

	// Sleep before fetching the initial info, so that the written mtime will be
	// considered reliable enough for caching by the getter.
	time.Sleep(3 * time.Second)

	g, err := NewGoPackagesModuleGetter(ctx, tempDir, "all")
	if err != nil {
		t.Fatal(err)
	}

	const fooPath = "foo.com/foo"
	foo1, err := g.Info(ctx, fooPath, "")
	if err != nil {
		t.Fatal(err)
	}
	foo2, err := g.Info(ctx, fooPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(foo1, foo2) {
		t.Errorf("Info(%q) returned inconsistent results: %v != %v", fooPath, foo1, foo2)
	}

	const barPath = "bar.com/bar"
	bar1, err := g.Info(ctx, barPath, "")
	if err != nil {
		t.Fatal(err)
	}
	bar2, err := g.Info(ctx, barPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(bar1, bar2) {
		t.Errorf("Info(%q) returned inconsistent results: %v != %v", barPath, bar1, bar2)
	}

	fpath := filepath.Join(tempDir, "foo", "foolog", "f.go")
	newContent := []byte("package foolog; const Log = 3")
	if err := os.WriteFile(fpath, newContent, 0600); err != nil {
		t.Fatal(err)
	}
	foo3, err := g.Info(ctx, fooPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Equal(foo1, foo3) {
		t.Errorf("Info(%q) results unexpectedly match: %v == %v", fooPath, foo1, foo3)
	}
	bar3, err := g.Info(ctx, barPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(bar1, bar2) {
		t.Errorf("Info(%q) returned inconsistent results: %v != %v", barPath, bar1, bar3)
	}
}

func TestEscapedPath(t *testing.T) {
	for _, test := range []struct {
		path, version, suffix string
		want                  string
	}{
		{
			"m.com", "v1", "info",
			"dir/cache/download/m.com/@v/v1.info",
		},
		{
			"github.com/aBc", "v2.3.4", "zip",
			"dir/cache/download/github.com/a!bc/@v/v2.3.4.zip",
		},
	} {
		g, err := NewModCacheGetter("dir")
		if err != nil {
			t.Fatal(err)
		}
		got, err := g.escapedPath(test.path, test.version, test.suffix)
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.Abs(test.want)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s, %s, %s: got %q, want %q", test.path, test.version, test.suffix, got, want)
		}
	}
}

func TestFSProxyGetter(t *testing.T) {
	ctx := context.Background()
	const (
		modulePath = "github.com/jackc/pgio"
		vers       = "v1.0.0"
		goMod      = "module github.com/jackc/pgio\n\ngo 1.12\n"
	)
	ts, err := time.Parse(time.RFC3339, "2019-03-30T17:04:38Z")
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewModCacheGetter("testdata/modcache")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("info", func(t *testing.T) {
		got, err := g.Info(ctx, modulePath, vers)
		if err != nil {
			t.Fatal(err)
		}
		want := &proxy.VersionInfo{Version: vers, Time: ts}
		if !cmp.Equal(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}

		// Asking for latest should give the same version.
		got, err = g.Info(ctx, modulePath, version.Latest)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}

		if _, err := g.Info(ctx, "nozip.com", vers); !errors.Is(err, derrors.NotFound) {
			t.Errorf("got %v, want NotFound", err)
		}
	})
	t.Run("mod", func(t *testing.T) {
		got, err := g.Mod(ctx, modulePath, vers)
		if err != nil {
			t.Fatal(err)
		}
		want := []byte(goMod)
		if !cmp.Equal(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}

		if _, err := g.Mod(ctx, "nozip.com", vers); !errors.Is(err, derrors.NotFound) {
			t.Errorf("got %v, want NotFound", err)
		}
	})
	t.Run("contentdir", func(t *testing.T) {
		fsys, err := g.ContentDir(ctx, modulePath, vers)
		if err != nil {
			t.Fatal(err)
		}
		// Just check that the go.mod file is there and has the right contents.
		f, err := fsys.Open("go.mod")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		want := []byte(goMod)
		if !cmp.Equal(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}

		if _, err := g.ContentDir(ctx, "nozip.com", vers); !errors.Is(err, derrors.NotFound) {
			t.Errorf("got %v, want NotFound", err)
		}
	})
}
