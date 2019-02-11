// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxyclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testCase struct {
	proxy  *httptest.Server
	client *Client
}

func setupTestCase(t *testing.T) (func(t *testing.T), *testCase) {
	proxy := httptest.NewServer(http.FileServer(http.Dir("testdata/modproxy/proxy")))
	tc := testCase{
		proxy:  proxy,
		client: New(proxy.URL),
	}

	fn := func(t *testing.T) {
		proxy.Close()
	}
	return fn, &tc
}

func TestCleanURL(t *testing.T) {
	for raw, expected := range map[string]string{
		"http://localhost:7000/index": "http://localhost:7000/index",
		"http://host.com/":            "http://host.com",
		"http://host.com///":          "http://host.com",
	} {
		got, err := cleanURL(raw)
		if err != nil {
			t.Errorf("cleanURL(%q) error: %v", raw, err)
		}
		if got != expected {
			t.Errorf("cleanURL(%q) = %q, want %q", raw, got, expected)
		}
	}
}

func TestCleanURLErrors(t *testing.T) {
	for _, raw := range []string{
		"localhost:7000/index",
		"host.com",
		"",
	} {
		_, err := cleanURL(raw)
		expectedErr := "is an invalid url"
		if err == nil {
			t.Errorf("cleanURL(%q) error = %v, want %q", raw, err, expectedErr)
		}
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("cleanURL(%q) error = %v, want %q", raw, err, expectedErr)
		}
	}
}

func TestGetInfo(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/module"
	version := "v1.0.0"
	info, err := testCase.proxyClient.GetInfo(name, version)
	if err != nil {
		t.Errorf("GetInfo(%q, %q) error: %v", name, version, err)
	}

	if info.Version != version {
		t.Errorf("VersionInfo.Version for GetInfo(%q, %q) = %q, want %q", name, version, info.Version, version)
	}

	expectedTime := time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC)
	if info.Time != expectedTime {
		t.Errorf("VersionInfo.Time for GetInfo(%q, %q) = %v, want %v", name, version, info.Time, expectedTime)
	}
}

func TestGetInfoVersionDoesNotExist(t *testing.T) {
	teardownTestCase, testCase := setupTestCase(t)
	defer teardownTestCase(t)

	name := "my/module"
	version := "v3.0.0"
	info, _ := testCase.proxyClient.GetInfo(name, version)
	if info != nil {
		t.Errorf("GetInfo(%q, %q) = %v, want %v", name, version, info, nil)
	}
}
