// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"
)

func TestSeriesPathForModule(t *testing.T) {
	for _, tc := range []struct {
		modulePath, wantSeriesPath string
	}{
		{
			modulePath:     "github.com/foo",
			wantSeriesPath: "github.com/foo",
		},
		{
			modulePath:     "github.com/foo/v2",
			wantSeriesPath: "github.com/foo",
		},
		{
			modulePath:     "std",
			wantSeriesPath: "std",
		},
		{
			modulePath:     "gopkg.in/russross/blackfriday.v2",
			wantSeriesPath: "gopkg.in/russross/blackfriday",
		},
	} {
		t.Run(tc.modulePath, func(t *testing.T) {
			if got := SeriesPathForModule(tc.modulePath); got != tc.wantSeriesPath {
				t.Errorf("SeriesPathForModule(%q) = %q; want = %q", tc.modulePath, got, tc.wantSeriesPath)
			}
		})
	}
}

func TestGoVersionForSemanticVersion(t *testing.T) {
	for _, tc := range []struct {
		name             string
		requestedVersion string
		want             string
		wantErr          bool
	}{
		{
			name:             "std version v1.12.5",
			requestedVersion: "v1.12.5",
			want:             "go1.12.5",
		},
		{
			name:             "std version v1.13, incomplete canonical version",
			requestedVersion: "v1.13",
			want:             "go1.13",
		},
		{
			name:             "std version v1.13.0-beta.1",
			requestedVersion: "v1.13.0-beta.1",
			want:             "go1.13beta1",
		},
		{
			name:             "std with digitless prerelease",
			requestedVersion: "v1.13.0-prerelease",
			want:             "go1.13prerelease",
		},
		{
			name:             "cmd version v1.13.0",
			requestedVersion: "v1.13.0",
			want:             "go1.13",
		},
		{
			name:             "bad std semver",
			requestedVersion: "v1.x",
			wantErr:          true,
		},
		{
			name:             "more bad std semver",
			requestedVersion: "v1.0-",
			wantErr:          true,
		},
		{
			name:             "bad prerelease",
			requestedVersion: "v1.13.0-beta1",
			wantErr:          true,
		},
		{
			name:             "another bad prerelease",
			requestedVersion: "v1.13.0-whatevs99",
			wantErr:          true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GoVersionForSemanticVersion(tc.requestedVersion)
			if (err != nil) != tc.wantErr {
				t.Errorf("GoVersionForSemanticVersion(%q) = %q, %v, wantErr %v", tc.requestedVersion, got, err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("GoVersionForSemanticVersion(%q) = %q, %v, wanted %q, %v", tc.requestedVersion, got, err, tc.want, nil)
			}
		})
	}
}
