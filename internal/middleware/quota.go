// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/golang/groupcache/lru"
	"golang.org/x/time/rate"
)

// Quota implements a simple IP-based rate limiter. Each set of incoming IP
// addresses with the same low-order byte gets qps requests per second, with the
// given burst (maximum requests per second; the size of the token bucket).
// Information is kept in an LRU cache of size maxEntries.
//
// If a request is disallowed, a 429 (TooManyRequests) will be served.
func Quota(qps, burst, maxEntries int) Middleware {
	cache := lru.New(maxEntries)
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := ipKey(r.Header.Get("X-Forwarded-For"))
			// key is empty if we couldn't parse an IP, or there is no IP.
			// Fail open in this case: allow serving.
			if key != "" {
				var limiter *rate.Limiter
				if v, ok := cache.Get(key); ok {
					limiter = v.(*rate.Limiter)
				} else {
					limiter = rate.NewLimiter(rate.Limit(qps), burst)
					cache.Add(key, limiter)
				}
				if !limiter.Allow() {
					const tmr = http.StatusTooManyRequests
					http.Error(w, http.StatusText(tmr), tmr)
					return
				}
			}
			h.ServeHTTP(w, r)
		})
	}
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
