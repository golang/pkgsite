// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/go-redis/redis/v7"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/discovery/internal/log"
)

var (
	keyCacheHit  = tag.MustNewKey("cache.hit")
	keyCacheName = tag.MustNewKey("cache.name")
	cacheResults = stats.Int64(
		"go-discovery/cache_result_count",
		"The result of a cache request.",
		stats.UnitDimensionless,
	)
	cacheErrors = stats.Int64(
		"go-discovery/cache_errors",
		"Errors retreiving from cache.",
		stats.UnitDimensionless,
	)

	// CacheResultCount is a counter of cache results, by cache name and hit success.
	CacheResultCount = &view.View{
		Name:        "custom.googleapis.com/go-discovery/cache/result_count",
		Measure:     cacheResults,
		Aggregation: view.Count(),
		Description: "cache results, by cache name and whether it was a hit",
		TagKeys:     []tag.Key{keyCacheName, keyCacheHit},
	}
	// CacheErrorCount is a counter of cache errors, by cache name.
	CacheErrorCount = &view.View{
		Name:        "custom.googleapis.com/go-discovery/cache/errors",
		Measure:     cacheErrors,
		Aggregation: view.Count(),
		Description: "cache errors, by cache name",
		TagKeys:     []tag.Key{keyCacheName},
	}
)

func recordCacheResult(ctx context.Context, name string, hit bool) {
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyCacheName, name),
		tag.Upsert(keyCacheHit, fmt.Sprint(hit)),
	}, cacheResults.M(1))
}

func recordCacheError(ctx context.Context, name string) {
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyCacheName, name),
	}, cacheErrors.M(1))
}

// Cache returns a new Middleware that times out each request after the given
// duration.
func Cache(name string, client *redis.Client, d time.Duration) Middleware {

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			key := r.URL.String()
			// set a short timeout for redis requests, so that we can quickly
			// fall-back to un-cached serving if redis is unavailable.
			getCtx, cancelGet := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancelGet()
			val, err := client.WithContext(getCtx).Get(key).Bytes()
			if err == redis.Nil {
				recordCacheResult(ctx, name, false)
			} else if err != nil {
				log.Errorf("client.Get(): %v", err)
			} else {
				recordCacheResult(ctx, name, true)
				content := bytes.NewReader(val)
				name := path.Base(r.URL.Path)
				http.ServeContent(w, r, name, time.Now(), content)
				return
			}
			rec := &cacheRecorder{ResponseWriter: w, buf: &bytes.Buffer{}}
			h.ServeHTTP(rec, r)
			if rec.bufErr == nil && (rec.statusCode == 0 || rec.statusCode == http.StatusOK) {
				log.Infof("caching response of length %d for %s", rec.buf.Len(), key)
				// write cache results asynchronously, since we don't want to slow down
				// our response.
				go func() {
					setCtx, cancelSet := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancelSet()
					_, err := client.WithContext(setCtx).Set(key, rec.buf.Bytes(), d).Result()
					if err != nil {
						recordCacheError(ctx, name)
						log.Errorf("client.Set(): %v", err)
					}
				}()
			}
		})
	}
}

// cacheRecorder is an http.ResponseWriter that collects http bytes for later
// writing to the cache. Along the way it collects any error, along with the
// resulting HTTP status code. We only cache 200 OK responses.
type cacheRecorder struct {
	http.ResponseWriter
	statusCode int

	bufErr error
	buf    *bytes.Buffer
}

func (r *cacheRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	if err == nil {
		_, bufErr := r.buf.Write(b)
		if bufErr != nil {
			r.bufErr = bufErr
		}
	} else {
		r.bufErr = fmt.Errorf("ResponseWriter.Write failed: %v", err)
	}
	return n, err
}

func (r *cacheRecorder) WriteHeader(statusCode int) {
	if statusCode > r.statusCode {
		// defensively take the largest status code that's written, so if any
		// middleware thinks the response is not OK, we will capture this.
		r.statusCode = statusCode
	}
	r.ResponseWriter.WriteHeader(statusCode)
}
