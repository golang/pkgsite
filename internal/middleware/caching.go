// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	icache "golang.org/x/pkgsite/internal/cache"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/cookie"
	"golang.org/x/pkgsite/internal/log"
)

var (
	keyCacheHit       = tag.MustNewKey("cache.hit")
	keyCacheName      = tag.MustNewKey("cache.name")
	keyCacheOperation = tag.MustNewKey("cache.operation")
	cacheResults      = stats.Int64(
		"go-discovery/cache/result_count",
		"The result of a cache request.",
		stats.UnitDimensionless,
	)
	cacheLatency = stats.Float64(
		"go-discovery/cache/result_latency",
		"Cache serving latency latency",
		stats.UnitMilliseconds,
	)
	cacheErrors = stats.Int64(
		"go-discovery/cache/errors",
		"Errors retrieving from cache.",
		stats.UnitDimensionless,
	)

	CacheResultCount = &view.View{
		Name:        "go-discovery/cache/result_count",
		Measure:     cacheResults,
		Aggregation: view.Count(),
		Description: "cache results, by cache name and whether it was a hit",
		TagKeys:     []tag.Key{keyCacheName, keyCacheHit},
	}
	CacheLatency = &view.View{
		Name:        "go-discovery/cache/result_latency",
		Measure:     cacheLatency,
		Description: "cache result latency, by cache name and whether it was a hit",
		Aggregation: ochttp.DefaultLatencyDistribution,
		TagKeys:     []tag.Key{keyCacheName, keyCacheHit},
	}
	CacheErrorCount = &view.View{
		Name:        "go-discovery/cache/errors",
		Measure:     cacheErrors,
		Aggregation: view.Count(),
		Description: "cache errors, by cache name",
		TagKeys:     []tag.Key{keyCacheName, keyCacheOperation},
	}

	// To avoid test flakiness, when TestMode is true, cache writes are
	// synchronous.
	TestMode = false
)

func recordCacheResult(ctx context.Context, name string, hit bool, latency time.Duration) {
	ms := float64(latency) / float64(time.Millisecond)
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyCacheName, name),
		tag.Upsert(keyCacheHit, strconv.FormatBool(hit)),
	}, cacheResults.M(1), cacheLatency.M(ms))
}

func recordCacheError(ctx context.Context, name, operation string) {
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyCacheName, name),
		tag.Upsert(keyCacheOperation, operation),
	}, cacheErrors.M(1))
}

type cache struct {
	name       string
	authValues []string
	cache      *icache.Cache
	delegate   http.Handler
	expirer    Expirer
}

// An Expirer computes the TTL that should be used when caching a page.
type Expirer func(r *http.Request) time.Duration

// TTL returns an Expirer that expires all pages after the given TTL.
func TTL(ttl time.Duration) Expirer {
	return func(r *http.Request) time.Duration {
		return ttl
	}
}

// Cache returns a new Middleware that caches every request.
// The name of the cache is used only for metrics.
// The expirer is a func that is used to map a new request to its TTL.
// authHeader is the header key used by the cache to know that a
// request should bypass the cache.
// authValues is the set of values that could be set on the authHeader in
// order to bypass the cache.
func Cache(name string, client *redis.Client, expirer Expirer, authValues []string) Middleware {
	return func(h http.Handler) http.Handler {
		return &cache{
			name:       name,
			authValues: authValues,
			cache:      icache.New(client),
			delegate:   h,
			expirer:    expirer,
		}
	}
}

func (c *cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check auth header to see if request should bypass cache.
	authVal := r.Header.Get(config.BypassCacheAuthHeader)
	for _, wantVal := range c.authValues {
		if authVal == wantVal {
			c.delegate.ServeHTTP(w, r)
			return
		}
	}
	// If the flash cookie is set, bypass the cache.
	if _, err := r.Cookie(cookie.AlternativeModuleFlash); err == nil {
		c.delegate.ServeHTTP(w, r)
		return
	}
	ctx := r.Context()
	key := r.URL.String()
	start := time.Now()
	reader, hit := c.get(ctx, key)
	recordCacheResult(ctx, c.name, hit, time.Since(start))
	if hit {
		if _, err := io.Copy(w, reader); err != nil {
			log.Errorf(ctx, "error copying zip bytes: %v", err)
		}
		return
	}
	rec := newRecorder(w)
	c.delegate.ServeHTTP(rec, r)
	if rec.bufErr == nil && (rec.statusCode == 0 || rec.statusCode == http.StatusOK) {
		ttl := c.expirer(r)
		if TestMode {
			c.put(ctx, key, rec, ttl)
		} else {
			go c.put(ctx, key, rec, ttl)
		}
	}
}

func (c *cache) get(ctx context.Context, key string) (io.Reader, bool) {
	// Set a short timeout for redis requests, so that we can quickly
	// fall back to un-cached serving if redis is unavailable.
	getCtx, cancelGet := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelGet()
	val, err := c.cache.Get(getCtx, key)
	if err != nil {
		select {
		case <-getCtx.Done():
			log.Infof(ctx, "cache get(%q): context timed out", key)
		default:
			log.Infof(ctx, "cache get(%q): %v", key, err)
		}
		recordCacheError(ctx, c.name, "GET")
		return nil, false
	}
	if val == nil {
		return nil, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(val))
	if err != nil {
		log.Errorf(ctx, "cache: gzip.NewReader: %v", err)
		recordCacheError(ctx, c.name, "UNZIP")
		return nil, false
	}
	return zr, true
}

func (c *cache) put(ctx context.Context, key string, rec *cacheRecorder, ttl time.Duration) {
	if err := rec.zipWriter.Close(); err != nil {
		log.Errorf(ctx, "cache: error closing zip for %q: %v", key, err)
		return
	}
	log.Infof(ctx, "caching response of length %d for %s", rec.buf.Len(), key)
	setCtx, cancelSet := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelSet()
	if err := c.cache.Put(setCtx, key, rec.buf.Bytes(), ttl); err != nil {
		recordCacheError(ctx, c.name, "SET")
		log.Warningf(ctx, "cache set %q: %v", key, err)
	}
}

func newRecorder(w http.ResponseWriter) *cacheRecorder {
	buf := &bytes.Buffer{}
	zw := gzip.NewWriter(buf)
	return &cacheRecorder{ResponseWriter: w, buf: buf, zipWriter: zw}
}

// cacheRecorder is an http.ResponseWriter that collects http bytes for later
// writing to the cache. Along the way it collects any error, along with the
// resulting HTTP status code. We only cache 200 OK responses.
type cacheRecorder struct {
	http.ResponseWriter
	statusCode int

	bufErr    error
	buf       *bytes.Buffer
	zipWriter *gzip.Writer
}

func (r *cacheRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	// Only try writing to the buffer if we haven't yet encountered an error.
	if r.bufErr == nil {
		if err == nil {
			zn, bufErr := r.zipWriter.Write(b)
			if bufErr != nil {
				r.bufErr = bufErr
			}
			if zn != n {
				r.bufErr = fmt.Errorf("wrote %d to zip, but wanted %d", zn, n)
			}
		} else {
			r.bufErr = fmt.Errorf("ResponseWriter.Write failed: %v", err)
		}
	}
	return n, err
}

func (r *cacheRecorder) WriteHeader(statusCode int) {
	if statusCode > r.statusCode {
		// Defensively take the largest status code that's written, so if any
		// middleware thinks the response is not OK, we will capture this.
		r.statusCode = statusCode
	}
	r.ResponseWriter.WriteHeader(statusCode)
}
