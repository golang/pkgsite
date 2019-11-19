// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/index"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/testing/testhelper"
)

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_integration_test", m, &testDB)
}

func TestEndToEndProcessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	var (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
		moduleData = map[string]string{
			"foo/foo.go": "package foo\n\nconst Foo = 525600",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
	)
	testVersions := []*proxy.TestVersion{proxy.NewTestVersion(t, modulePath, version, moduleData)}
	proxyClient, indexClient, teardownClients := setupProxyAndIndex(t, testVersions...)
	defer teardownClients()

	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: s.Addr()})

	// TODO(b/143760329): it would be better if InMemoryQueue made http requests
	// back to ETL, rather than calling fetch itself.
	queue := etl.NewInMemoryQueue(ctx, proxyClient, testDB, 10)

	etlServer, err := etl.NewServer(testDB, indexClient, proxyClient, nil, queue, nil, "../../../content/static")
	if err != nil {
		t.Fatal(err)
	}
	etlMux := http.NewServeMux()
	etlServer.Install(etlMux.Handle)
	etlHTTP := httptest.NewServer(etlMux)

	frontendServer, err := frontend.NewServer(testDB, nil, "../../../content/static", false)
	if err != nil {
		t.Fatal(err)
	}
	frontendMux := http.NewServeMux()
	frontendServer.Install(frontendMux.Handle, redisClient)
	frontendHTTP := httptest.NewServer(frontendMux)

	etlURL := etlHTTP.URL + "/poll-and-queue"
	etlResp, err := http.Get(etlURL)
	if err != nil {
		t.Fatal(err)
	}
	defer etlResp.Body.Close()
	if etlResp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: got status %d, want %d", etlURL, etlResp.StatusCode, http.StatusOK)
	}
	// TODO(b/143760329): This should really be made deterministic.
	time.Sleep(100 * time.Millisecond)
	queue.WaitForTesting(ctx)

	frontendURL := frontendHTTP.URL + "/github.com/my/module/foo"
	frontendResp, err := http.Get(frontendURL)
	if err != nil {
		t.Fatal(err)
	}
	defer frontendResp.Body.Close()
	if frontendResp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: got status %d, want %d", frontendURL, frontendResp.StatusCode, http.StatusOK)
	}
	bodyBytes, err := ioutil.ReadAll(frontendResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if idx := strings.Index(body, "525600"); idx < 0 {
		t.Error("Documentation constant 525600 not found in body")
	}
}

func setupProxyAndIndex(t *testing.T, versions ...*proxy.TestVersion) (*proxy.Client, *index.Client, func()) {
	t.Helper()
	proxyClient, teardownProxy := proxy.SetupTestProxy(t, versions)
	var indexVersions []*internal.IndexVersion
	for _, v := range versions {
		indexVersions = append(indexVersions, &internal.IndexVersion{
			Path:      v.ModulePath,
			Version:   v.Version,
			Timestamp: time.Now(),
		})
	}
	indexClient, teardownIndex := index.SetupTestIndex(t, indexVersions)
	teardown := func() {
		teardownProxy()
		teardownIndex()
	}
	return proxyClient, indexClient, teardown
}
