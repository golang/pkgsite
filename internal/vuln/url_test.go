// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"net/url"
	"runtime"
	"testing"
)

func TestURLToFilePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows is not supported (see convertFileURLPath")
	}
	for _, tc := range urlTests {
		if tc.url == "" {
			continue
		}
		tc := tc

		t.Run(tc.url, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tc.url, err)
			}

			path, err := URLToFilePath(u)
			if err != nil {
				if err.Error() == tc.wantErr {
					return
				}
				if tc.wantErr == "" {
					t.Fatalf("urlToFilePath(%v): %v; want <nil>", u, err)
				} else {
					t.Fatalf("urlToFilePath(%v): %v; want %s", u, err, tc.wantErr)
				}
			}

			if path != tc.filePath || tc.wantErr != "" {
				t.Fatalf("urlToFilePath(%v) = %q, <nil>; want %q, %s", u, path, tc.filePath, tc.wantErr)
			}
		})
	}
}

type urlTest struct {
	url          string
	filePath     string
	canonicalURL string // If empty, assume equal to url.
	wantErr      string
}

var urlTests = []urlTest{
	// Examples from RFC 8089:
	{
		url:      `file:///path/to/file`,
		filePath: `/path/to/file`,
	},
	{
		url:          `file:/path/to/file`,
		filePath:     `/path/to/file`,
		canonicalURL: `file:///path/to/file`,
	},
	{
		url:          `file://localhost/path/to/file`,
		filePath:     `/path/to/file`,
		canonicalURL: `file:///path/to/file`,
	},

	// We reject non-local files.
	{
		url:     `file://host.example.com/path/to/file`,
		wantErr: "file URL specifies non-local host",
	},
}
