// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package serverconfig

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/config"
)

func TestValidateAppVersion(t *testing.T) {
	for _, test := range []struct {
		in      string
		wantErr bool
	}{
		{"", true},
		{"20190912t130708", false},
		{"20190912t130708x", true},
		{"2019-09-12t13-07-0400", false},
		{"2019-09-12t13070400", true},
		{"2019-09-11t22-14-0400-2f4680648b319545c55c6149536f0a74527901f6", false},
	} {
		err := ValidateAppVersion(test.in)
		if (err != nil) != test.wantErr {
			t.Errorf("ValidateAppVersion(%q) = %v, want error = %t", test.in, err, test.wantErr)
		}
	}
}

func TestChooseN(t *testing.T) {
	tests := []struct {
		configVar string
		n         int
		wantMatch []string
	}{
		{"foo", 2, []string{"foo", ""}},
		{"foo1 \n foo2", 1, []string{"^foo[12]$"}},
		{"foo1 \n foo2", 2, []string{"^foo[12]$", "^foo[12]$"}},
		{"foo1 foo2", 4, []string{"^foo[12]$", "^foo[12]$", "", ""}},
		{"foo1\nfoo2\nfoo3", 5, []string{"^foo[123]$", "^foo[123]$", "^foo[123]$", "", ""}},
	}
	for _, test := range tests {
		gots := chooseN(test.configVar, test.n)

		if len(gots) != test.n {
			t.Errorf("chooseN must return a slice of n(%v), got %v", test.n, len(gots))
		}
		seen := make(map[string]struct{}, test.n)

		allMatch := true
		allUnique := true
		for i, got := range gots {
			if got != "" {
				_, ok := seen[got]
				allUnique = allUnique && !ok

				seen[got] = struct{}{}
			}

			matched, err := regexp.MatchString(test.wantMatch[i], got)
			if err != nil {
				t.Fatal(err)
			}
			allMatch = allMatch && matched

			seen[got] = struct{}{}
		}
		if !allMatch {
			t.Errorf("chooseN(%q, %v) = %v, want matches %v", test.configVar, test.n, gots, test.wantMatch)
		}
		if !allUnique {
			t.Errorf("chooseN(%q, %v) = %v, want all unique", test.configVar, test.n, gots)
		}
	}
}

func TestProcessOverrides(t *testing.T) {
	tr := true
	f := false
	cfg := config.Config{
		DBHost: "origHost",
		DBName: "origName",
		Quota:  config.QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 3, RecordOnly: &tr},
	}
	ov := `
        DBHost: newHost
        Quota:
           MaxEntries: 17
           RecordOnly: false
    `
	processOverrides(context.Background(), &cfg, []byte(ov))
	got := cfg
	want := config.Config{
		DBHost: "newHost",
		DBName: "origName",
		Quota:  config.QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 17, RecordOnly: &f},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(config.Config{})); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestParseCommaList(t *testing.T) {
	for _, test := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"foo", []string{"foo"}},
		{"foo,bar", []string{"foo", "bar"}},
		{" foo, bar ", []string{"foo", "bar"}},
		{",, ,foo ,  , bar,,,", []string{"foo", "bar"}},
	} {
		got := parseCommaList(test.in)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%q: got %#v, want %#v", test.in, got, test.want)
		}
	}
}

func TestEnvAndApp(t *testing.T) {
	for _, test := range []struct {
		serviceID string
		wantEnv   string
		wantApp   string
	}{
		{"default", "prod", "frontend"},
		{"exp-worker", "exp", "worker"},
		{"-foo-bar", "unknownEnv", "foo-bar"},
		{"", "local", "unknownApp"},
	} {
		cfg := &config.Config{ServiceID: test.serviceID}
		gotEnv := cfg.DeploymentEnvironment()
		if gotEnv != test.wantEnv {
			t.Errorf("%q: got %q, want %q", test.serviceID, gotEnv, test.wantEnv)
		}
		gotApp := cfg.Application()
		if gotApp != test.wantApp {
			t.Errorf("%q: got %q, want %q", test.serviceID, gotApp, test.wantApp)
		}
	}
}
