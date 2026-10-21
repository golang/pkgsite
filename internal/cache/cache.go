// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cache implements a redis-based page cache
// for pkgsite.
package cache

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"golang.org/x/pkgsite/internal/derrors"
)

// Cache is a Redis-based cache.
type Cache struct {
	client *redis.Client
}

// New creates a new Cache using the given Redis client.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// Get returns the value for key,  or nil if the key does not exist.
func (c *Cache) Get(ctx context.Context, key string) (value []byte, err error) {
	defer derrors.Wrap(&err, "Get(%q)", key)
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil { // not found
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

// Put inserts the key with the given data and time-to-live.
func (c *Cache) Put(ctx context.Context, key string, data []byte, ttl time.Duration) (err error) {
	defer derrors.Wrap(&err, "Put(%q, data, %s)", key, ttl)
	_, err = c.client.Set(ctx, key, data, ttl).Result()
	return err
}

// Clear deletes all entries from the cache.
func (c *Cache) Clear(ctx context.Context) (err error) {
	defer derrors.Wrap(&err, "Clear()")
	status := c.client.FlushAll(ctx)
	return status.Err()
}

// Delete deletes the given keys. It does not return an error if a key does not
// exist.
func (c *Cache) Delete(ctx context.Context, keys ...string) (err error) {
	defer derrors.Wrap(&err, "Delete(%q)", keys)
	cmd := c.client.Unlink(ctx, keys...) // faster, asynchronous delete
	return cmd.Err()
}

// DeletePrefix deletes all keys beginning with prefix.
func (c *Cache) DeletePrefix(ctx context.Context, prefix string) (err error) {
	defer derrors.Wrap(&err, "DeletePrefix(%q)", prefix)
	iter := c.client.Scan(ctx, 0, prefix+"*", int64(scanCount)).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) > scanCount {
			if err := c.Delete(ctx, keys...); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if iter.Err() != nil {
		return iter.Err()
	}
	if len(keys) > 0 {
		return c.Delete(ctx, keys...)
	}
	return nil
}

// The "count" argument to the Redis SCAN command, which is a hint for how much
// work to perform.
// Also used as the batch size for Delete calls in DeletePrefix.
// var for testing.
var scanCount = 100
