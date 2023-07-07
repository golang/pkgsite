// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// github.com/alicebob/miniredis/v2 pulls in
// github.com/yuin/gopher-lua which uses a non
// build-tag-guarded use of the syscall package.
//go:build !plan9

package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestIPKey(t *testing.T) {
	for _, test := range []struct {
		in   string
		want any
	}{
		{"", ""},
		{"1.2.3", ""},
		{"128.197.17.3", "128.197.17.0"},
		{"  128.197.17.3, foo  ", "128.197.17.0"},
		{"2001:db8::ff00:42:8329", "2001:db8::ff00:42:8300"},
	} {
		got := ipKey(test.in)
		if got != test.want {
			t.Errorf("%q: got %v, want %v", test.in, got, test.want)
		}
	}
}

func TestEnforceQuota(t *testing.T) {
	// This test is inherently time-dependent, so inherently flaky, especially on CI.
	// So run it a few times before giving up.
	ctx := context.Background()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer c.Close()

	const qps = 5

	var failReason string
	for n := 0; n < 10; n++ {
		failReason = ""

		check := func(n int, ip string, want bool) {
			if failReason != "" {
				return
			}
			for i := 0; i < n; i++ {
				blocked, reason := enforceQuota(ctx, c, qps, ip+",x", []byte{1, 2, 3, 4})
				got := !blocked
				if got != want {
					failReason = fmt.Sprintf("%d: got %t, want %t (reason=%q)", i, got, want, reason)
					break
				}
			}
		}

		check(qps, "1.2.3.4", true) // first qps requests are allowed
		check(1, "1.2.3.4", false)  // anything after that fails
		check(1, "1.2.3.5", false)  // low-order byte doesn't matter
		check(qps, "1.2.4.1", true) // other IP is allowed
		check(1, "1.2.4.9", false)  // other IP blocked after qps requests

		if failReason == "" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Error(failReason)
}
