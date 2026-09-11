// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package serverconfig

import (
	"context"
	"regexp"
	"testing"
	"time"

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
		wantLen   int
		wantMatch string
	}{
		{"foo", 2, 1, "^foo$"},
		{"foo1 \n foo2", 1, 1, "^foo[12]$"},
		{"foo1 \n foo2", 2, 2, "^foo[12]$"},
		{"foo1 foo2", 4, 2, "^foo[12]$"},
		{"foo1\nfoo2\nfoo3", 5, 3, "^foo[123]$"},
		{"", 2, 0, ""},
	}
	for _, test := range tests {
		gots := chooseN(test.configVar, test.n)

		if len(gots) != test.wantLen {
			t.Errorf("chooseN(%q, %v) returned %d entries, want %d", test.configVar, test.n, len(gots), test.wantLen)
			continue
		}
		seen := make(map[string]struct{}, len(gots))
		for _, got := range gots {
			matched, err := regexp.MatchString(test.wantMatch, got)
			if err != nil {
				t.Fatal(err)
			}
			if !matched {
				t.Errorf("chooseN(%q, %v) = %v, want each to match %v", test.configVar, test.n, gots, test.wantMatch)
			}
			if _, ok := seen[got]; ok {
				t.Errorf("chooseN(%q, %v) = %v, want all unique", test.configVar, test.n, gots)
			}
			seen[got] = struct{}{}
		}
	}
}

func TestProcessOverrides(t *testing.T) {
	tr := true
	f := false
	cfg := config.Config{
		DBHosts: []string{"origHost1", "origHost2"},
		DBName:  "origName",
		Quota:   config.QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 3, RecordOnly: &tr},
	}
	ov := `
        DBHost: newHost1 newHost2
        Quota:
           MaxEntries: 17
           RecordOnly: false
    `
	processOverrides(context.Background(), &cfg, []byte(ov))
	got := cfg
	want := config.Config{
		DBHosts: []string{"newHost1", "newHost2"},
		DBName:  "origName",
		Quota:   config.QuotaSettings{QPS: 1, Burst: 2, MaxEntries: 17, RecordOnly: &f},
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
func TestInitPoolSettings(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name     string
		envs     map[string]string
		wantOpen int
		wantIdle int
		wantLife time.Duration
	}{
		{
			name: "overridden",
			envs: map[string]string{
				"GO_DISCOVERY_DATABASE_HOST":              "localhost",
				"GO_DISCOVERY_DATABASE_MAX_OPEN_CONNS":    "42",
				"GO_DISCOVERY_DATABASE_MAX_IDLE_CONNS":    "13",
				"GO_DISCOVERY_DATABASE_CONN_MAX_LIFETIME": "10m",
			},
			wantOpen: 42,
			wantIdle: 13,
			wantLife: 10 * time.Minute,
		},
		{
			name: "defaults",
			envs: map[string]string{
				"GO_DISCOVERY_DATABASE_HOST": "localhost",
			},
			wantOpen: config.DefaultDBMaxOpenConns,
			wantIdle: config.DefaultDBMaxIdleConns,
			wantLife: config.DefaultDBConnMaxLifetime,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for k, v := range test.envs {
				t.Setenv(k, v)
			}
			cfg, err := Init(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DBMaxOpenConns != test.wantOpen {
				t.Errorf("DBMaxOpenConns: got %d, want %d", cfg.DBMaxOpenConns, test.wantOpen)
			}
			if cfg.DBMaxIdleConns != test.wantIdle {
				t.Errorf("DBMaxIdleConns: got %d, want %d", cfg.DBMaxIdleConns, test.wantIdle)
			}
			if cfg.DBConnMaxLifetime != test.wantLife {
				t.Errorf("DBConnMaxLifetime: got %s, want %s", cfg.DBConnMaxLifetime, test.wantLife)
			}
		})
	}
}
