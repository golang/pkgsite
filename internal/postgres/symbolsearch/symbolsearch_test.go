// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbolsearch

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestGenerateQuery ensure that go generate was run and the generated queries
// are up to date with the raw queries.
func TestGenerateQuery(t *testing.T) {
	for _, test := range []struct {
		name, q, want string
	}{
		{"querySymbol", rawQuerySymbol, QuerySymbol},
		{"queryPackageDotSymbol", rawQueryPackageDotSymbol, QueryPackageDotSymbol},
		{"queryOneDot", rawQueryOneDot, QueryOneDot},
		{"queryMultiWord", rawQueryMultiWord, QueryMultiWord},
	} {
		t.Run(test.name, func(t *testing.T) {
			if diff := cmp.Diff(test.want, test.q); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
