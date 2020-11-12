// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/complete"
)

func TestAutoCompletion(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := redis.NewClient(&redis.Options{Addr: s.Addr()})
	// For convenence, populate completion data based on detail path -> imports.
	pathData := map[string]int{
		"foo.com/bar@v1.2.3/baz":          123,
		"foo.com/quux@v1.0.0/bark":        10,
		"github.com/something@v2.0.0/foo": 80,
	}
	for k, v := range pathData {
		got, err := parseDetailsURLPath(k)
		if err != nil {
			t.Fatal(err)
		}
		partial := complete.Completion{
			PackagePath: got.fullPath,
			ModulePath:  got.modulePath,
			Version:     got.requestedVersion,
			Importers:   v,
		}
		completions := complete.PathCompletions(partial)
		var zs []*redis.Z
		for _, cmpl := range completions {
			zs = append(zs, &redis.Z{Member: cmpl.Encode()})
		}
		if v > 0 {
			r.ZAdd(ctx, complete.PopularKey, zs...)
		} else {
			r.ZAdd(ctx, complete.RemainingKey, zs...)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []struct {
		q    string
		want []string
	}{
		{"baz", []string{"foo.com/bar/baz"}},
		{"bar", []string{"foo.com/bar/baz", "foo.com/quux/bark"}},
		{"foo", []string{"github.com/something/foo", "foo.com/bar/baz"}},
	}
	for _, test := range tests {
		t.Run(test.q, func(t *testing.T) {
			results, err := doCompletion(ctx, r, test.q, 2)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, res := range results {
				got = append(got, res.PackagePath)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("doCompletion(%q) mismatch (-want +got)\n%s", test.q, diff)
			}
		})
	}
}

func TestNextPrefix(t *testing.T) {
	tests := []struct {
		prefix, want string
	}{
		{"", ""},
		{"~~~", ""},
		{"aa", "ab"},
		{"aB", "aC"},
		{"a~", "b"},
	}
	for _, test := range tests {
		if got := nextPrefix(test.prefix); got != test.want {
			t.Errorf("nextPrefix(%q) = %q, want %q", test.prefix, got, test.want)
		}
	}
}
