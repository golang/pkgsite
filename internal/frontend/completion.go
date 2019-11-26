// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-redis/redis/v7"
	"golang.org/x/discovery/internal/complete"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// handleAutoCompletion handles requests for /autocomplete?q=<input prefix>, by
// querying redis sorted sets indexing package paths.
func (s *Server) handleAutoCompletion(w http.ResponseWriter, r *http.Request) {
	var completions []*complete.Completion
	if s.cmplClient != nil {
		var err error
		q := r.FormValue("q")
		completions, err = doCompletion(r.Context(), s.cmplClient, strings.ToLower(q), 5)
		if err != nil {
			code := http.StatusInternalServerError
			http.Error(w, http.StatusText(code), code)
			return
		}
	}
	if completions == nil {
		// autocomplete.js complains if the JSON returned by this endpoint is null,
		// so we initialize a non-nil empty array to serialize to an empty JSON
		// array.
		completions = []*complete.Completion{}
	}
	response, err := json.Marshal(completions)
	if err != nil {
		log.Errorf("error marshalling completion: json.Marshal: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, bytes.NewReader(response)); err != nil {
		log.Errorf("Error copying json buffer to ResponseWriter: %v", err)
	}
}

// scoredCompletion wraps Completions with a relevancy score, so that they can
// be sorted.
type scoredCompletion struct {
	c     *complete.Completion
	score int
}

// doCompletion executes the completion query against redis. This is inspired
// by http://oldblog.antirez.com/post/autocomplete-with-redis.html, but
// improved as follows:
//  + Use ZRANGEBYLEX to avoid storing each possible prefix, since that was
//    added to Redis since the original blog post.
//  + Use an additional sorted set that holds popular packages, to improve
//    completion relevancy.
//
// We autocomplete the query 'q' as follows
//  1. Query for popular completions starting with q using ZRANGEBYLEX (more
//     details on this below). We fetch an arbitrary number of results (1000)
//     to bound the amount of work done by redis.
//  2. Sort the returned completions by our score (a mix of popularity and
//     proximity to the end of the import path), and filter to the top
//     maxResults.
//  3. If we have maxResults results, we're done. Otherwise do (1) on the index
//     of remaining (unpopular) package paths, add to our result set, and sort
//     again (because unpopular packages might actually score higher than
//     popular packages).
func doCompletion(ctx context.Context, r *redis.Client, q string, maxResults int) (_ []*complete.Completion, err error) {
	defer derrors.Wrap(&err, "doCompletion(%q, %d)", q, maxResults)
	scored, err := completeWithIndex(ctx, r, q, complete.PopularKey, maxResults)
	if err != nil {
		return nil, err
	}
	if len(scored) < maxResults {
		unpopular, err := completeWithIndex(ctx, r, q, complete.RemainingKey, maxResults-len(scored))
		if err != nil {
			return nil, err
		}
		scored = append(scored, unpopular...)
		// Re-sort, as it is possible that an unpopular completion actually has a
		// higher score than a popular completion due to the weighting for suffix
		// length.
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
	}
	var completions []*complete.Completion
	for _, s := range scored {
		completions = append(completions, s.c)
	}
	return completions, nil
}

func completeWithIndex(ctx context.Context, r *redis.Client, q, indexKey string, maxResults int) (_ []*scoredCompletion, err error) {
	defer derrors.Wrap(&err, "completeWithIndex(%q, %q, %d)", q, indexKey, maxResults)

	// Query for possible completions using ZRANGEBYLEX. See documentation at
	// https://redis.io/commands/zrangebylex
	// Notably, the "(" character in the Min and Max fields means 'exclude this
	// endpoint'.
	// We bound our search in two ways: (1) by setting Max to the smallest string
	// that lexically greater than q but does not start with q, and (2) by
	// setting an arbitrary limit of 1000 results.
	entries, err := r.WithContext(ctx).ZRangeByLex(indexKey, &redis.ZRangeBy{
		Min:   "(" + q,
		Max:   "(" + nextPrefix(q),
		Count: 1000,
	}).Result()
	var scored []*scoredCompletion
	for _, entry := range entries {
		c, err := complete.Decode(entry)
		if err != nil {
			return nil, err
		}
		offset := len(strings.Split(entry, "/"))
		s := &scoredCompletion{
			c: c,
			// Weight importers by distance of the matching text from the end of the
			// import path. This is done in an attempt to make results more relevant
			// the closer the match is to the end of the import path. For example, if
			// the user types 'net', we should have some preference for 'net' over
			// 'net/http'. In this case, it actually works out like so:
			//  - net has ~68000 importers
			//  - net/http has ~130000 importers
			//
			// So the score of 'net' is ~68000 (offset=1), and the score of
			// 'net/http' is ~65000 (130K/2, as offset=2), therefore net should be
			// sorted above 'net/http' in the results.
			//
			// This heuristic is a total guess, but since this is just autocomplete
			// it probably doesn't matter much. In testing, it felt like autocomplete
			// was completing the packages I wanted.
			//
			// The `- offset` term is added to break ties in the case where all
			// completion results have 0 importers.
			score: c.Importers/offset - offset,
		}
		scored = append(scored, s)
	}
	// sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}
	return scored, nil
}

// nextPrefix returns the first string (according to lexical sorting) that is
// greater than prefix but does not start with prefix.
func nextPrefix(prefix string) string {
	// redis strings are ASCII. Note that among printing ASCII characters '!' has
	// the smallest byte value and '~' has the largest byte value. It also so
	// happens that these are both valid characters in a URL.
	if prefix == "" {
		return ""
	}
	lastChar := prefix[len(prefix)-1]
	if lastChar >= '~' {
		// If the last character is '~', there is no greater ascii character so we
		// must move to the previous character to find a lexically greater string
		// that doesn't start with prefix. Note that in the degenerate case where
		// prefix is nothing but twiddles (e.g. "~~~"), we will recurse until we return "",
		// which is acceptable: there is no prefix that satisfies our requirements:
		// all strings greater than "~~~" must also start with "~~~"
		return nextPrefix(prefix[:len(prefix)-1])
	}
	return prefix[:len(prefix)-1] + string(lastChar+1)
}
