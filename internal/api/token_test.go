// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"testing"
	"time"
)

func TestPageToken(t *testing.T) {
	t.Setenv("K_SERVICE", "test-service")

	tests := []int{0, 1, 42, 100, 999999}

	for _, n := range tests {
		encoded, err := encodePageToken(n)
		if err != nil {
			t.Fatalf("encodePageToken(%d): %v", n, err)
		}
		t.Logf("encoded token for %d: %s", n, encoded)

		decoded, err := decodePageToken(encoded)
		if err != nil {
			t.Fatalf("decodePageToken(%s): %v", encoded, err)
		}

		if decoded != n {
			t.Errorf("decoded = %d, want %d", decoded, n)
		}
	}
}

func TestDecodePageTokenInvalid(t *testing.T) {
	t.Setenv("K_SERVICE", "test-service")

	invalidTokens := []string{
		"not-hex",
		"1234",                             // too short
		"00000000000000000000000000000000", // garbage decrypted bytes
	}

	for _, token := range invalidTokens {
		if _, err := decodePageToken(token); err == nil {
			t.Errorf("decodePageToken(%s) succeeded, want error", token)
		}
	}

	// Test expired token (49 hours ago).
	expiredTime := time.Now().Add(-49 * time.Hour)
	expiredToken, err := encodePageToken1(42, expiredTime)
	if err != nil {
		t.Fatalf("encodePageToken1: %v", err)
	}
	if _, err := decodePageToken(expiredToken); err == nil {
		t.Error("decodePageToken succeeded for expired token, want error")
	} else if want := "expired"; err.Error() != want {
		t.Errorf("decodePageToken error = %v, want %s", err, want)
	}

	// Test future token (2 minutes in the future).
	futureTime := time.Now().Add(2 * time.Minute)
	futureToken, err := encodePageToken1(42, futureTime)
	if err != nil {
		t.Fatalf("encodePageToken1: %v", err)
	}
	if _, err := decodePageToken(futureToken); err == nil {
		t.Error("decodePageToken succeeded for future token, want error")
	} else if want := "from the future"; err.Error() != want {
		t.Errorf("decodePageToken error = %v, want %s", err, want)
	}

	// Test overly large offset (1,000,000).
	largeOffsetToken, err := encodePageToken1(maxOffset+1, time.Now())
	if err != nil {
		t.Fatalf("encodePageToken1: %v", err)
	}
	if _, err := decodePageToken(largeOffsetToken); err == nil {
		t.Error("decodePageToken succeeded for large offset token, want error")
	} else if want := "offset too large"; err.Error() != want {
		t.Errorf("decodePageToken error = %v, want %s", err, want)
	}
}

func TestStringPageToken(t *testing.T) {
	t.Setenv("K_SERVICE", "test-service")

	tests := []string{
		"",
		"a",
		"hello",
		"github.com/google/go-cmp/cmp",
		"golang.org/x/pkgsite/internal/postgres/details_test",
		"a/very/long/path/that/exceeds/the/aes/block/size/and/requires/gcm/to/encrypt/properly/without/truncation/or/errors/because/it/is/long",
	}

	for _, s := range tests {
		encoded, err := encodeStringPageToken(s)
		if err != nil {
			t.Fatalf("encodeStringPageToken(%q): %v", s, err)
		}
		t.Logf("encoded string token for %q: %s (len=%d)", s, encoded, len(encoded))

		decoded, err := decodeStringPageToken(encoded)
		if err != nil {
			t.Fatalf("decodeStringPageToken(%s): %v", encoded, err)
		}

		if decoded != s {
			t.Errorf("decoded = %q, want %q", decoded, s)
		}
	}
}

func TestDecodeStringPageTokenInvalid(t *testing.T) {
	t.Setenv("K_SERVICE", "test-service")

	invalidTokens := []string{
		"not-hex",
		"1234",                             // too short
		"00000000000000000000000000000000", // garbage decrypted bytes
	}

	for _, token := range invalidTokens {
		if _, err := decodeStringPageToken(token); err == nil {
			t.Errorf("decodeStringPageToken(%s) succeeded, want error", token)
		}
	}

	// Test expired token (49 hours ago).
	expiredTime := time.Now().Add(-tokenExpiry)
	expiredToken, err := encodeStringPageToken1("expired-test", expiredTime)
	if err != nil {
		t.Fatalf("encodeStringPageToken1: %v", err)
	}
	if _, err := decodeStringPageToken(expiredToken); err == nil {
		t.Error("decodeStringPageToken succeeded for expired token, want error")
	} else if want := "expired"; err.Error() != want {
		t.Errorf("decodeStringPageToken error = %v, want %s", err, want)
	}

	// Test future token (2 minutes in the future).
	futureTime := time.Now().Add(2 * time.Minute)
	futureToken, err := encodeStringPageToken1("future-test", futureTime)
	if err != nil {
		t.Fatalf("encodeStringPageToken1: %v", err)
	}
	if _, err := decodeStringPageToken(futureToken); err == nil {
		t.Error("decodeStringPageToken succeeded for future token, want error")
	} else if want := "from the future"; err.Error() != want {
		t.Errorf("decodeStringPageToken error = %v, want %s", err, want)
	}
}
