// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package natural implements "[Natural Sort Order]" for strings. This allows
// sorting strings in a way that numbers in strings are compared numerically,
// rather than lexicographically.
//
// [Natural Sort Order]: https://en.wikipedia.org/wiki/Natural_sort_order
package natural

import (
	"cmp"
	"strings"
)

// Compare implements [natural sort order] for strings, where numbers inside
// strings are compared numerically. For example:
//
//	"uint8" < "uint16" < "uint32"
//
// The implementation conceptually splits the string into components of digits
// and non-digits. Non-digit sequences are compared lexicographically.
// Digit sequences are compared numerically. When numeric values are equal, the
// one with fewer leading zeros is considered smaller. For example:
//
//	"01" < "001" < "02"
//
// The numeric components consist only of sequences of decimal digits [0-9]
// denoting non-negative integers. For example:
//
//	"1e6"  < "10e5"
//	"0xAB" < "0xB"
//	"-5"   < "-10"
//
// [natural sort order]: https://en.wikipedia.org/wiki/Natural_sort_order
func Compare(a, b string) int {
	for {
		prefix := nondigitCommonPrefixLength(a, b)
		a, b = a[prefix:], b[prefix:]
		if a == "" || b == "" {
			return cmp.Compare(len(a), len(b))
		}

		adig := isdigit(a[0])
		bdig := isdigit(b[0])

		// digit vs non-digit?
		// The one with the digit is smaller because its non-digit sequence is shorter.
		if adig != bdig {
			return -boolToSign(adig)
		}
		// Inv: adig == bdig

		// If both are non-digits, compare lexicographically.
		if !adig {
			return -boolToSign(a[0] < b[0])
		}
		// Inv: adig && bdig

		// Both are numbers, so we compare them numerically.
		ac, azeros := countDigits(a)
		bc, bzeros := countDigits(b)

		// If one has more non-zero digits then it's obviously larger.
		if ac-azeros != bc-bzeros {
			return cmp.Compare(ac-azeros, bc-bzeros)
		}

		// Comparing equal-length digit strings will give the
		// same result as converting them to numbers.
		r := strings.Compare(a[azeros:ac], b[bzeros:bc])
		if r != 0 {
			return r
		}

		// The one with fewer leading zeros is smaller.
		if azeros != bzeros && azeros+bzeros > 0 {
			return cmp.Compare(azeros, bzeros)
		}

		// They were equal, so continue.
		a, b = a[ac:], b[bc:]
	}
}

// boolToSign converts a boolean to an integer, 1 or -1.
func boolToSign(b bool) int {
	if b {
		return 1
	} else {
		return -1
	}
}

// Less implements natural string comparison, where numbers are compared numerically.
func Less(a, b string) bool {
	return Compare(a, b) < 0
}

// nondigitCommonPrefixLength returns the length of longest common non-digit
// byte-oriented prefix of a and b.
//
//	nondigitCommonPrefixLength("a1", "a1") == 1
//	nondigitCommonPrefixLength("ab", "ac") == 1
func nondigitCommonPrefixLength(a, b string) (i int) {
	for i = 0; i < len(a) && i < len(b) && a[i] == b[i] && !isdigit(a[i]); i++ {
	}
	return i
}

// countDigits returns the number of prefix digits and leading zeros in s.
func countDigits(s string) (count, leadingZeros int) {
	foundNonZero := false
	for i, c := range []byte(s) {
		if !isdigit(c) {
			return i, leadingZeros
		}
		if !foundNonZero && c == '0' {
			leadingZeros++
		} else {
			foundNonZero = true
		}
	}
	return len(s), leadingZeros
}

// isdigit returns true if c is a digit.
func isdigit(c byte) bool {
	return '0' <= c && c <= '9'
}
