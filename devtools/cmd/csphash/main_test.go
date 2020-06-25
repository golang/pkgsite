// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScriptStartRegexp(t *testing.T) {
	for _, test := range []struct {
		in   string
		want bool
	}{
		{`<script>`, true},
		{`<Script>`, true},
		{`<script src="/static/min.js">`, true},
		{`<script
					integrity="sha256-xyz"
					src=/foo
			  >`, true},
		{`<scriptify>`, false},
		{`<enscript>`, false},
	} {
		got := scriptStartRegexp.MatchString(test.in)
		if got != test.want {
			t.Errorf("%s: got %t, want %t", test.in, got, test.want)
		}
	}
}

func TestScriptsReader(t *testing.T) {
	in := `
		<script>foo </script>
		<script src="/static/min.js"></script>
	`
	got, err := scriptsReader([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []*script{
		{
			tag:  []byte(`<script>`),
			body: []byte(`foo `),
		},
		{
			tag:  []byte(`<script src="/static/min.js">`),
			body: []byte(""),
		},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(script{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}
