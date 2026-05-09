// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// tokenCipher computes a cipher.Block from the K_SERVICE environment variable.
var tokenCipher = sync.OnceValues(func() (cipher.Block, error) {
	// K_SERVICE is the name of the Cloud Run service. It isn't
	// exactly a secret, but it's also not known to users. If
	// someone does manage to guess it or break the encryption
	// (for integer tokens we are using only one block and there
	// are many easily guessable plaintexts), it's not the end of
	// the world. And anyone who does that much work to de-obfuscate
	// a token surely knows they are doing something wrong.
	service := os.Getenv("K_SERVICE")
	if service == "" {
		return nil, errors.New("K_SERVICE is not set")
	}
	key := sha256.Sum256([]byte(service))
	return aes.NewCipher(key[:])
})

// encodePageToken obfuscates a page token by binary encoding the integer n and
// the current timestamp, encrypting it with AES using K_SERVICE as a key, and
// hex encoding the result.
func encodePageToken(n int) (string, error) {
	return encodePageToken1(n, time.Now())
}

// encodePageToken1 is like encodePageToken but allows passing a specific time for testing.
func encodePageToken1(n int, t time.Time) (string, error) {
	// 1. Binary encode the timestamp and the int.
	// AES block size is 16 bytes. The high-order 8 bytes contain the Unix timestamp,
	// and the low-order 8 bytes contain the int n.
	src := make([]byte, aes.BlockSize)
	binary.BigEndian.PutUint64(src[:8], uint64(t.Unix()))
	binary.BigEndian.PutUint64(src[8:], uint64(n))

	// 2. Compute AES on it.
	block, err := tokenCipher()
	if err != nil {
		return "", err
	}

	dst := make([]byte, aes.BlockSize)
	block.Encrypt(dst, src)

	// 3. Hex encode the result.
	return hex.EncodeToString(dst), nil
}

const (
	tokenExpiry = 48 * time.Hour
	maxOffset   = 1e6
)

// decodePageToken reverses the obfuscation of a page token, returning the original integer n.
// It rejects tokens older than tokenExpiry.
func decodePageToken(token string) (int, error) {
	// 1. Hex decode the token.
	src, err := hex.DecodeString(token)
	if err != nil {
		return 0, err
	}
	if len(src) != aes.BlockSize {
		return 0, errors.New("invalid length")
	}

	// 2. Compute AES decryption.
	block, err := tokenCipher()
	if err != nil {
		return 0, err
	}

	dst := make([]byte, aes.BlockSize)
	block.Decrypt(dst, src)

	// 3. Binary decode the result.
	timestamp := int64(binary.BigEndian.Uint64(dst[:8]))
	n := binary.BigEndian.Uint64(dst[8:])

	// Reject expired tokens.
	tokenTime := time.Unix(timestamp, 0)
	if time.Since(tokenTime) > tokenExpiry {
		return 0, errors.New("expired")
	}
	// Reject tokens from the future (by more than 1 minute, allowing for clock skew).
	if time.Since(tokenTime) < -time.Minute {
		return 0, errors.New("from the future")
	}
	// Reject overly large offsets.
	if n > maxOffset {
		return 0, errors.New("offset too large")
	}
	in := int(n)
	if in < 0 {
		return 0, errors.New("negative offset")
	}
	return in, nil
}

// encodeStringPageToken obfuscates a string page token by prepending the current
// timestamp, encrypting it with AES-GCM using K_SERVICE as a key, and hex
// encoding the result.
func encodeStringPageToken(s string) (string, error) {
	return encodeStringPageToken1(s, time.Now())
}

// encodeStringPageToken1 is like encodeStringPageToken but allows passing a specific time for testing.
func encodeStringPageToken1(s string, t time.Time) (string, error) {
	block, err := tokenCipher()
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Pack timestamp (8 bytes) + string
	plaintext := make([]byte, 8+len(s))
	binary.BigEndian.PutUint64(plaintext[:8], uint64(t.Unix()))
	copy(plaintext[8:], s)

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	// Result is nonce + ciphertext
	result := append(nonce, ciphertext...)
	return hex.EncodeToString(result), nil
}

// decodeStringPageToken reverses the obfuscation of a string page token, returning the original string.
// It rejects tokens older than tokenExpiry.
func decodeStringPageToken(token string) (string, error) {
	src, err := hex.DecodeString(token)
	if err != nil {
		return "", err
	}
	block, err := tokenCipher()
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := aesgcm.NonceSize()
	if len(src) < nonceSize {
		return "", errors.New("token too short")
	}
	nonce, ciphertext := src[:nonceSize], src[nonceSize:]
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	if len(plaintext) < 8 {
		return "", errors.New("invalid plaintext")
	}
	timestamp := int64(binary.BigEndian.Uint64(plaintext[:8]))
	s := string(plaintext[8:])

	// Reject expired tokens.
	tokenTime := time.Unix(timestamp, 0)
	if time.Since(tokenTime) > tokenExpiry {
		return "", errors.New("expired")
	}
	// Reject tokens from the future.
	if time.Since(tokenTime) < -time.Minute {
		return "", errors.New("from the future")
	}
	return s, nil
}
