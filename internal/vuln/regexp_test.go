// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import "testing"

func TestCanonicalGoID(t *testing.T) {
	tests := []struct {
		id     string
		wantID string
		wantOK bool
	}{
		{
			id:     "GO-1999-0001",
			wantID: "GO-1999-0001",
			wantOK: true,
		},
		{
			id:     "GO-1999-000111",
			wantID: "GO-1999-000111",
			wantOK: true,
		},
		{
			id:     "go-1999-0001",
			wantID: "GO-1999-0001",
			wantOK: true,
		},
		{
			id:     "GO-1999",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "GHSA-cfgh-2345-rwxq",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "CVE-1999-000123",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "ghsa-Cfgh-2345-Rwxq",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "cve-1999-000123",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "cve-ghsa-go",
			wantID: "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			gotID, gotOK := CanonicalGoID(tt.id)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("CanonicalGoID(%s) = (%s, %t), want (%s, %t)", tt.id, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestCanonicalAlias(t *testing.T) {
	tests := []struct {
		id     string
		wantID string
		wantOK bool
	}{
		{
			id:     "GO-1999-0001",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "GHSA-cfgh-2345-rwxq",
			wantID: "GHSA-cfgh-2345-rwxq",
			wantOK: true,
		},
		{
			id:     "CVE-1999-000123",
			wantID: "CVE-1999-000123",
			wantOK: true,
		},
		{
			id:     "go-1999-0001",
			wantID: "",
			wantOK: false,
		},
		{
			id:     "ghsa-Cfgh-2345-Rwxq",
			wantID: "GHSA-cfgh-2345-rwxq",
			wantOK: true,
		},
		{
			id:     "cve-1999-000123",
			wantID: "CVE-1999-000123",
			wantOK: true,
		},
		{
			id:     "abc-CVE-1999-0001",
			wantID: "",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			gotID, gotOK := CanonicalAlias(tt.id)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("CanonicalAlias(%s) = (%s, %t), want (%s, %t)", tt.id, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}
