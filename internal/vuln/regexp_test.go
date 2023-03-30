// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import "testing"

func TestIsGoID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{
			id:   "GO-1999-0001",
			want: true,
		},
		{
			id:   "GO-2023-12345678",
			want: true,
		},
		{
			id:   "GO-2023-123",
			want: false,
		},
		{
			id:   "GO-abcd-0001",
			want: false,
		},
		{
			id:   "CVE-1999-0001",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := IsGoID(tt.id)
			if got != tt.want {
				t.Errorf("IsGoID(%s) = %t, want %t", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsAlias(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{
			id:   "GO-1999-0001",
			want: false,
		},
		{
			id:   "GHSA-abcd-1234-efgh",
			want: true,
		},
		{
			id:   "CVE-1999-000123",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := IsAlias(tt.id)
			if got != tt.want {
				t.Errorf("IsAlias(%s) = %t, want %t", tt.id, got, tt.want)
			}
		})
	}
}
