// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/groupcache/lru"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/time/rate"
)

var (
	keyQuotaBlocked = tag.MustNewKey("quota.blocked")
	quotaResults    = stats.Int64(
		"go-discovery/quota_result_count",
		"The result of a quota check.",
		stats.UnitDimensionless,
	)
	// QuotaResultCount is a counter of quota results, by whether the request was blocked or not.
	QuotaResultCount = &view.View{
		Name:        "go-discovery/quota/result_count",
		Measure:     quotaResults,
		Aggregation: view.Count(),
		Description: "quota results, by blocked or allowed",
		TagKeys:     []tag.Key{keyQuotaBlocked},
	}
)

// Quota implements a simple IP-based rate limiter. Each set of incoming IP
// addresses with the same low-order byte gets qps requests per second, with the
// given burst.
// Information is kept in an LRU cache of size maxEntries.
//
// If a request is disallowed, a 429 (TooManyRequests) will be served.
func Quota(settings config.QuotaSettings) Middleware {
	var mu sync.Mutex
	cache := lru.New(settings.MaxEntries)

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, url := range settings.AcceptedURLs {
				if r.Referer() == url {
					recordQuotaMetric("accepted")
					log.Infof(r.Context(), "Quota: accepting referer %q", r.Referer())
					h.ServeHTTP(w, r)
					return
				}
			}

			key := ipKey(r.Header.Get("X-Forwarded-For"))
			// key is empty if we couldn't parse an IP, or there is no IP.
			// Fail open in this case: allow serving.
			var limiter *rate.Limiter
			if key != "" {
				mu.Lock()
				if v, ok := cache.Get(key); ok {
					limiter = v.(*rate.Limiter)
				} else {
					limiter = rate.NewLimiter(rate.Limit(settings.QPS), settings.Burst)
					cache.Add(key, limiter)
				}
				mu.Unlock()
			}
			blocked := limiter != nil && !limiter.Allow()
			recordQuotaMetric(strconv.FormatBool(blocked))
			if blocked && settings.RecordOnly != nil && !*settings.RecordOnly {
				const tmr = http.StatusTooManyRequests
				http.Error(w, http.StatusText(tmr), tmr)
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}

func recordQuotaMetric(blocked string) {
	stats.RecordWithTags(context.Background(), []tag.Mutator{
		tag.Upsert(keyQuotaBlocked, blocked),
	}, quotaResults.M(1))
}

func ipKey(s string) string {
	fields := strings.SplitN(s, ",", 2)
	// First field is the originating IP address.
	origin := strings.TrimSpace(fields[0])
	ip := net.ParseIP(origin)
	if ip == nil {
		return ""
	}
	// Zero out last byte, to cover ranges.
	ip[len(ip)-1] = 0
	return ip.String()
}
