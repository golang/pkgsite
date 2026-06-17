// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"testing"
)

func TestDBConnInfo(t *testing.T) {
	testCases := []struct {
		name              string
		secondaryHost     string
		wantConnInfo      string
		wantSecondaryConn string
	}{
		{
			name:              "with secondary host",
			secondaryHost:     "hostB",
			wantConnInfo:      "user='testuser' password='testpass' host='hostA,hostB' port=5432 dbname='testdb' sslmode='disable' connect_timeout=5 options='-c statement_timeout=1800000'",
			wantSecondaryConn: "user='testuser' password='testpass' host='hostB,hostA' port=5432 dbname='testdb' sslmode='disable' connect_timeout=5 options='-c statement_timeout=1800000'",
		},
		{
			name:              "without secondary host",
			secondaryHost:     "",
			wantConnInfo:      "user='testuser' password='testpass' host='hostA' port=5432 dbname='testdb' sslmode='disable' connect_timeout=5 options='-c statement_timeout=1800000'",
			wantSecondaryConn: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				DBUser:          "testuser",
				DBPassword:      "testpass",
				DBHost:          "hostA",
				DBPort:          "5432",
				DBName:          "testdb",
				DBSSL:           "disable",
				DBSecondaryHost: tc.secondaryHost,
			}

			if got := cfg.DBConnInfo(); got != tc.wantConnInfo {
				t.Errorf("DBConnInfo() = %q, want %q", got, tc.wantConnInfo)
			}
			if got := cfg.DBSecondaryConnInfo(); got != tc.wantSecondaryConn {
				t.Errorf("DBSecondaryConnInfo() = %q, want %q", got, tc.wantSecondaryConn)
			}
		})
	}
}
