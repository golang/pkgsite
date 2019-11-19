// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal/complete"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

const popularCutoff = 50

// handleUpdateRedisIndexes scans recently modified search documents, and
// updates redis auto completion indexes with data from these documents.
func (s *Server) handleUpdateRedisIndexes(w http.ResponseWriter, r *http.Request) {
	err := updateRedisIndexes(r.Context(), s.db.GetSQLDB(), s.redisClient, popularCutoff)
	if err != nil {
		log.Errorf("error updating redis indexes: %v", err)
		code := http.StatusInternalServerError
		http.Error(w, http.StatusText(code), code)
		return
	}
	fmt.Fprint(w, "OK")
}

// updateRedisIndexes updates redisClient with autocompletion data from db.
// cutoff specifies the number of importers at which a package is considered
// popular, and is passed-in as an argument to facilitate testing.
func updateRedisIndexes(ctx context.Context, db *sql.DB, redisClient *redis.Client, cutoff int) (err error) {
	defer derrors.Wrap(&err, "updateRedisIndexes")
	if redisClient == nil {
		return errors.New("redis client is nil")
	}

	// For autocompletion, we track two separate "indexes" (sorted sets of
	// package path suffixes): one for popular packages, and one for the
	// remainder, as defined by the popularCutoff const.  This allows us to
	// suggest popular completions, even when the user input is short (i.e. we
	// want to suggest 'fmt' when the user types 'f', but don't want to scan all
	// completions that start with the letter 'f').
	//
	// This function scans search documents in the database and builds up a
	// pipeline that writes these two sorted sets to Redis, using timestamped
	// temporary keys, and then renames them to the keys used by the frontend for
	// autocompletion.
	//
	// See https://redis.io/commands/rename for more information on renaming:
	// it's unclear whether renaming is atomic, but we don't really care.
	// Populating these indexes currently takes 1-2 minutes, and renaming takes
	// 1-2 seconds. Even if completions are broken during this 1-2 seconds, it's
	// preferable to them being broken for 1-2 minutes. We could do something
	// more clever, such as updating the completion data in place using
	// ZREMRANGEBYLEX followed by ZADD, but that would be significantly more
	// complicated.
	//
	// One additional concern of this operation is that we temporary double the
	// size of our redis database while we're staging the new completion data.
	// That's fine, but it's dangerous if we ever have a bug and this operation
	// was either not cleaned up properly, or run concurrently. In light of this,
	// we first look for evidence of another update operation currently running,
	// by scanning Redis for keys that match the temporary key pattern.

	query := `
		SELECT package_path, module_path, version, imported_by_count
		FROM search_documents`
	var args []interface{}

	// Here we use the *sql.DB directly, rather than a function on postgres.DB,
	// so that we can write to our redis pipeline while we stream results from
	// the DB. Otherwise, we would have to:
	//  - add a method on postgres.DB for the trivial query above
	//  - add a type (or reuse SearchResult) to hold the subset of search
	//    document data used here.
	//  - hold two copies of all search results in memory while building the
	//    redis pipeline below.
	//
	// It seemed cleaner to expose the sql.DB here.
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("db.QueryContext(%q): %v", query, err)
	}
	defer rows.Close()
	pipe := redisClient.Pipeline()
	defer pipe.Close()

	// Check for an ongoing update operation, as described above.
	tempKeyPattern := fmt.Sprintf("%s*-*", complete.KeyPrefix)
	existing, _, err := redisClient.Scan(0, tempKeyPattern, 1).Result()
	if err != nil {
		return fmt.Errorf(`redis error: Scan(%q): %v`, tempKeyPattern, err)
	}
	if len(existing) > 0 {
		return fmt.Errorf("found existing in-progress completion index: %v", existing[0])
	}

	// Use temporary timestamped keys while we write the completion data, as it
	// can take ~minutes.
	keyPop := fmt.Sprintf("%s-%s", complete.PopularKey, time.Now().Format(time.RFC3339))
	keyRem := fmt.Sprintf("%s-%s", complete.RemainingKey, time.Now().Format(time.RFC3339))

	// Always clean up: DEL succeeds even if the keys have been renamed.
	defer func() {
		if _, err := redisClient.Del(keyPop).Result(); err != nil {
			log.Errorf("redisClient.Del(%q): %v", keyPop, err)
		}
		if _, err := redisClient.Del(keyRem).Result(); err != nil {
			log.Errorf("redisClient.Del(%q): %v", keyRem, err)
		}
	}()

	// Build up a Redis pipeline as we scan the search_documents table. If needed
	// this pipeline could be intermittently flushed, but in testing it was
	// fastest to use one single pipeline.
	for rows.Next() {
		// partial holds everything but the completion suffix.
		var partial complete.Completion
		if err := rows.Scan(&partial.PackagePath, &partial.ModulePath, &partial.Version, &partial.Importers); err != nil {
			return fmt.Errorf("rows.Scan: %v", err)
		}
		cmpls := complete.PathCompletions(partial)
		var zs []*redis.Z
		for _, cmpl := range cmpls {
			zs = append(zs, &redis.Z{Member: cmpl.Encode()})
		}
		switch {
		case partial.Importers >= cutoff:
			pipe.ZAdd(keyPop, zs...)
		default:
			pipe.ZAdd(keyRem, zs...)
		}
	}
	if _, err := pipe.ExecContext(ctx); err != nil {
		return fmt.Errorf("redis error: pipe.Exec: %v", err)
	}
	if _, err := redisClient.Rename(keyPop, complete.PopularKey).Result(); err != nil {
		return fmt.Errorf(`redis error: Rename(%q, %q): %v`, keyPop, complete.PopularKey, err)
	}
	if _, err := redisClient.Rename(keyRem, complete.RemainingKey).Result(); err != nil {
		return fmt.Errorf(`redis error: Rename(%q, %q): %v`, keyRem, complete.RemainingKey, err)
	}
	return nil
}
