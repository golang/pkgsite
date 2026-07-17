// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !plan9

package static

import (
	"testing"
)

func TestExtractLicense(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "standard",
			input:   "/*\nCopyright 2000 */",
			want:    "\nCopyright 2000 */",
			wantErr: false,
		},
		{
			name:    "exclamation",
			input:   "/*!\nCopyright 2021 The Go Authors. All rights reserved.\n */",
			want:    "\nCopyright 2021 The Go Authors. All rights reserved.\n */",
			wantErr: false,
		},
		{
			name:    "double asterisk",
			input:   "/**\n * Copyright 2026\n */",
			want:    "\n * Copyright 2026\n */",
			wantErr: false,
		},
		{
			name:    "leading whitespace",
			input:   "   \n/*\nCopyright 2000 */",
			want:    "\nCopyright 2000 */",
			wantErr: false,
		},
		{
			name:    "error: no comment",
			input:   "var x = 1",
			wantErr: true,
		},
		{
			name:    "error: empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "error: comment not at start",
			input:   "var x = 1 /*\nCopyright 2000 */",
			wantErr: true,
		},
		{
			name:    "error: line comment instead of block comment",
			input:   "// Copyright 2000",
			wantErr: true,
		},
		{
			name:    "error: unclosed block comment",
			input:   "/*\nCopyright 2000",
			wantErr: true,
		},
		{
			name:    "error: initial comment missing copyright",
			input:   "/* Hello World */",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractLicense([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Fatalf("extractLicense(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("extractLicense(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
