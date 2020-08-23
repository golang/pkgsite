// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/sync/errgroup"
)

type annotation struct {
	error
	msg string
}

// doIndexPage writes the status page. On error it returns the error and a short
// string to be written back to the client.
func (s *Server) doIndexPage(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "doIndexPage")
	var (
		stats       *postgres.VersionStats
		experiments []*internal.Experiment
		excluded    []string
	)
	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() error {
		var err error
		stats, err = s.db.GetVersionStats(ctx)
		if err != nil {
			return annotation{err, "error fetching stats"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		experiments, err = s.db.GetExperiments(ctx)
		if err != nil {
			return annotation{err, "error fetching experiments"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		excluded, err = s.db.GetExcludedPrefixes(ctx)
		if err != nil {
			return annotation{err, "error fetching excluded"}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		var e annotation
		if errors.As(err, &e) {
			log.Errorf(ctx, e.msg, err)
		}
		return err
	}

	type count struct {
		Code  int
		Desc  string
		Count int
	}
	var counts []*count
	for code, n := range stats.VersionCounts {
		c := &count{Code: code, Count: n}
		if e := derrors.FromStatus(code, ""); e != nil && e != derrors.Unknown {
			c.Desc = e.Error()
		}
		counts = append(counts, c)
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].Code < counts[j].Code })
	page := struct {
		Config          *config.Config
		Env             string
		ResourcePrefix  string
		LatestTimestamp *time.Time
		Counts          []*count
		LocationID      string
		Experiments     []*internal.Experiment
		Excluded        []string
	}{
		Config:          s.cfg,
		Env:             env(s.cfg.ServiceID),
		ResourcePrefix:  strings.ToLower(env(s.cfg.ServiceID)) + "-",
		LatestTimestamp: &stats.LatestTimestamp,
		Counts:          counts,
		LocationID:      s.cfg.LocationID,
		Experiments:     experiments,
		Excluded:        excluded,
	}
	return renderPage(ctx, w, page, s.templates[indexTemplate])
}

func (s *Server) doVersionsPage(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "doVersionsPage")
	const pageSize = 20
	g, ctx := errgroup.WithContext(r.Context())
	var next, failures, recents []*internal.ModuleVersionState
	g.Go(func() error {
		var err error
		next, err = s.db.GetNextModulesToFetch(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching next versions"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		failures, err = s.db.GetRecentFailedVersions(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching recent failures"}
		}
		return nil
	})
	g.Go(func() error {
		var err error
		recents, err = s.db.GetRecentVersions(ctx, pageSize)
		if err != nil {
			return annotation{err, "error fetching recent versions"}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		var e annotation
		if errors.As(err, &e) {
			log.Errorf(ctx, e.msg, err)
		}
		return err
	}
	page := struct {
		Next, Recent, RecentFailures []*internal.ModuleVersionState
		Config                       *config.Config
		Env                          string
		ResourcePrefix               string
	}{
		Next:           next,
		Recent:         recents,
		RecentFailures: failures,
		Config:         s.cfg,
		Env:            env(s.cfg.ServiceID),
		ResourcePrefix: strings.ToLower(env(s.cfg.ServiceID)) + "-",
	}
	return renderPage(ctx, w, page, s.templates[versionsTemplate])
}

func env(serviceID string) string {
	if serviceID == "" {
		return "Local"
	}
	parts := strings.Split(serviceID, "-")
	if len(parts) == 1 {
		return "Prod"
	}
	return strings.ToUpper(parts[0][:1]) + parts[0][1:]
}

func renderPage(ctx context.Context, w http.ResponseWriter, page interface{}, tmpl *template.Template) (err error) {
	defer derrors.Wrap(&err, "renderPage")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, page); err != nil {
		return err
	}
	if _, err := io.Copy(w, &buf); err != nil {
		log.Errorf(ctx, "Error copying buffer to ResponseWriter: %v", err)
		return err
	}
	return nil
}
