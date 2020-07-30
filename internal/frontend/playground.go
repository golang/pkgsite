// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"io"
	"net/http"
	"strconv"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/pkgsite/internal/log"
)

// playgroundURL is the playground endpoint used for share links.
const playgroundURL = "https://play.golang.org"

var (
	keyPlaygroundShareStatus = tag.MustNewKey("playground.share.status")
	playgroundShareStatus    = stats.Int64(
		"go-discovery/playground_share_count",
		"The status of a request to play.golang.org/share",
		stats.UnitDimensionless,
	)

	PlaygroundShareRequestCount = &view.View{
		Name:        "go-discovery/playground/share_count",
		Measure:     playgroundShareStatus,
		Aggregation: view.Count(),
		Description: "Playground share request count",
		TagKeys:     []tag.Key{keyPlaygroundShareStatus},
	}
)

// handlePlay handles requests that mirror play.golang.org/share.
func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	makeFetchPlayRequest(w, r, playgroundURL)
}

func httpErrorStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func makeFetchPlayRequest(w http.ResponseWriter, r *http.Request, pgURL string) {
	ctx := r.Context()
	if r.Method != http.MethodPost {
		httpErrorStatus(w, http.StatusMethodNotAllowed)
		return
	}
	req, err := http.NewRequest("POST", pgURL+"/share", r.Body)
	if err != nil {
		log.Errorf(ctx, "ERROR share error: %v", err)
		httpErrorStatus(w, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req = req.WithContext(r.Context())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf(ctx, "ERROR share error: %v", err)
		httpErrorStatus(w, http.StatusInternalServerError)
		return
	}
	stats.RecordWithTags(r.Context(),
		[]tag.Mutator{tag.Upsert(keyPlaygroundShareStatus, strconv.Itoa(resp.StatusCode))},
		playgroundShareStatus.M(int64(resp.StatusCode)),
	)
	copyHeader := func(k string) {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	copyHeader("Content-Type")
	copyHeader("Content-Length")
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Errorf(ctx, "ERROR writing shareId: %v", err)
	}
}
