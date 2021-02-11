// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/cache"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/index"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/queue"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/worker"
)

var (
	testDB      *postgres.DB
	testModules []*proxy.Module
)

func TestMain(m *testing.M) {
	dochtml.LoadTemplates(template.TrustedSourceFromConstant("../../../content/static/html/doc"))
	testModules = proxy.LoadTestModules("../../proxy/testdata")
	postgres.RunDBTests("discovery_integration_test", m, &testDB)
}

func TestEndToEndProcessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	proxyClient, proxyServer, indexClient, teardownClients := setupProxyAndIndex(t)
	defer teardownClients()

	redisCache, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer redisCache.Close()
	redisCacheClient := redis.NewClient(&redis.Options{Addr: redisCache.Addr()})

	redisHA, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer redisHA.Close()
	redisHAClient := redis.NewClient(&redis.Options{Addr: redisHA.Addr()})

	fetcher := &worker.Fetcher{
		ProxyClient:  proxyClient,
		SourceClient: source.NewClient(1 * time.Second),
		DB:           testDB,
		Cache:        cache.New(redisCacheClient),
	}
	// TODO: it would be better if InMemory made http requests
	// back to worker, rather than calling fetch itself.
	queue := queue.NewInMemory(ctx, 10, nil, func(ctx context.Context, mpath, version string) (int, error) {
		code, _, err := fetcher.FetchAndUpdateState(ctx, mpath, version, "test", false)
		return code, err
	})
	workerServer, err := worker.NewServer(&config.Config{}, worker.ServerConfig{
		DB:               testDB,
		IndexClient:      indexClient,
		ProxyClient:      proxyClient,
		SourceClient:     source.NewClient(1 * time.Second),
		RedisHAClient:    redisHAClient,
		RedisCacheClient: redisCacheClient,
		Queue:            queue,
		StaticPath:       template.TrustedSourceFromConstant("../../../content/static"),
	})
	if err != nil {
		t.Fatal(err)
	}
	workerMux := http.NewServeMux()
	workerServer.Install(workerMux.Handle)
	workerHTTP := httptest.NewServer(workerMux)

	frontendHTTP := setupFrontend(ctx, t, queue, redisCacheClient)
	if _, err := doGet(workerHTTP.URL + "/poll"); err != nil {
		t.Fatal(err)
	}
	// TODO: This should really be made deterministic.
	time.Sleep(100 * time.Millisecond)
	if _, err := doGet(workerHTTP.URL + "/enqueue"); err != nil {
		t.Fatal(err)
	}
	// TODO: This should really be made deterministic.
	time.Sleep(100 * time.Millisecond)
	queue.WaitForTesting(ctx)

	var wantKeys []string
	for _, test := range []struct {
		url, want string
	}{
		{"example.com/basic", "v1.1.0"},
		{"example.com/basic@v1.0.0", "v1.0.0"},
		{"example.com/single", "This is the README"},
		{"example.com/single/pkg", "hello"},
		{"example.com/single@v1.0.0/pkg", "hello"},
	} {
		wantKeys = append(wantKeys, "/"+test.url)
		body, err := doGet(frontendHTTP.URL + "/" + test.url)
		if err != nil {
			t.Fatalf("%s: %v", test.url, err)
		}
		if !strings.Contains(string(body), test.want) {
			t.Errorf("%q not found in body", test.want)
			t.Logf("%s", body)
		}
	}

	// Test cache invalidation.
	keys := cacheKeys(t, redisCacheClient)
	sort.Strings(wantKeys)
	if !cmp.Equal(keys, wantKeys) {
		t.Errorf("cache keys: got %v, want %v", keys, wantKeys)
	}

	// Process a newer version of a module, and verify that the cache has been invalidated.
	modulePath := "example.com/single"
	version := "v1.2.3"
	proxyServer.AddModule(proxy.FindModule(testModules, modulePath, "v1.0.0").ChangeVersion(version))
	_, _, err = fetcher.FetchAndUpdateState(ctx, modulePath, version, "test", false)
	if err != nil {
		t.Fatal(err)
	}

	// All the keys with modulePath should be gone, but the others should remain.
	keys = cacheKeys(t, redisCacheClient)
	wantKeys = []string{"/example.com/basic", "/example.com/basic@v1.0.0"}
	if !cmp.Equal(keys, wantKeys) {
		t.Errorf("cache keys: got %v, want %v", keys, wantKeys)
	}
}

func cacheKeys(t *testing.T, client *redis.Client) []string {
	keys, err := client.Keys(context.Background(), "*").Result()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys)
	return keys
}

// doGet executes an HTTP GET request for url and returns the response body, or
// an error if anything went wrong or the response status code was not 200 OK.
func doGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http.Get(%q): %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http.Get(%q): status: %d, want %d", url, resp.StatusCode, http.StatusOK)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll(): %v", err)
	}
	return body, nil
}

func setupProxyAndIndex(t *testing.T) (*proxy.Client, *proxy.Server, *index.Client, func()) {
	t.Helper()
	proxyServer := proxy.NewServer(testModules)
	proxyClient, teardownProxy, err := proxy.NewClientForServer(proxyServer)
	if err != nil {
		t.Fatal(err)
	}

	var indexVersions []*internal.IndexVersion
	for _, m := range testModules {
		indexVersions = append(indexVersions, &internal.IndexVersion{
			Path:      m.ModulePath,
			Version:   m.Version,
			Timestamp: time.Now(),
		})
	}
	indexClient, teardownIndex := index.SetupTestIndex(t, indexVersions)
	teardown := func() {
		teardownProxy()
		teardownIndex()
	}
	return proxyClient, proxyServer, indexClient, teardown
}
