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
	"os"
	"runtime"
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

var startTime = time.Now()

// doIndexPage writes the status page. On error it returns the error and a short
// string to be written back to the client.
func (s *Server) doIndexPage(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "doIndexPage")
	var (
		experiments []*internal.Experiment
		excluded    []string
	)
	if s.getExperiments != nil {
		experiments = s.getExperiments()
	}
	g, ctx := errgroup.WithContext(r.Context())
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

	var gms runtime.MemStats
	runtime.ReadMemStats(&gms)
	sms, err := getSystemMemStats()
	if err != nil {
		log.Errorf(ctx, "could not get system stats: %v", err)
	}
	pms, err := getProcessMemStats()
	if err != nil {
		log.Errorf(ctx, "could not get process stats: %v", err)
	}

	var logsURL string
	if s.cfg.OnGKE() {
		env := s.cfg.DeploymentEnvironment()
		cluster := env + "-" + "pkgsite"
		logsURL = `https://pantheon.corp.google.com/logs/query;query=resource.type%3D%22k8s_container%22%20resource.labels.cluster_name%3D%22` +
			cluster +
			`%22%20resource.labels.container_name%3D%22worker%22?project=` +
			s.cfg.ProjectID
	} else {
		logsURL = `https://cloud.google.com/console/logs/viewer?resource=gae_app%2Fmodule_id%2F` + s.cfg.ServiceID + `&project=` +
			s.cfg.ProjectID
	}

	page := struct {
		Config          *config.Config
		Env             string
		ResourcePrefix  string
		LatestTimestamp *time.Time
		LocationID      string
		Hostname        string
		StartTime       time.Time
		Experiments     []*internal.Experiment
		Excluded        []string
		LoadShedStats   LoadShedStats
		GoMemStats      runtime.MemStats
		ProcessStats    processMemStats
		SystemStats     systemMemStats
		CgroupStats     map[string]uint64
		Fetches         []*FetchInfo
		LogsURL         string
		DBInfo          *postgres.UserInfo
	}{
		Config:         s.cfg,
		Env:            env(s.cfg),
		ResourcePrefix: strings.ToLower(env(s.cfg)) + "-",
		LocationID:     s.cfg.LocationID,
		Hostname:       os.Getenv("HOSTNAME"),
		StartTime:      startTime,
		Experiments:    experiments,
		Excluded:       excluded,
		LoadShedStats:  ZipLoadShedStats(),
		GoMemStats:     gms,
		ProcessStats:   pms,
		SystemStats:    sms,
		CgroupStats:    getCgroupMemStats(),
		Fetches:        FetchInfos(),
		LogsURL:        logsURL,
		DBInfo:         s.workerDBInfo(),
	}
	return renderPage(ctx, w, page, s.templates[indexTemplate])
}

func (s *Server) doVersionsPage(w http.ResponseWriter, r *http.Request) (err error) {
	defer derrors.Wrap(&err, "doVersionsPage")
	const pageSize = 20
	g, ctx := errgroup.WithContext(r.Context())
	var (
		next, failures, recents []*internal.ModuleVersionState
		stats                   *postgres.VersionStats
	)
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
	g.Go(func() error {
		var err error
		stats, err = s.db.GetVersionStats(ctx)
		if err != nil {
			return annotation{err, "error fetching stats"}
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
		Next, Recent, RecentFailures []*internal.ModuleVersionState
		Config                       *config.Config
		Env                          string
		ResourcePrefix               string
		LatestTimestamp              *time.Time
		Counts                       []*count
	}{
		Next:            next,
		Recent:          recents,
		RecentFailures:  failures,
		Config:          s.cfg,
		Env:             env(s.cfg),
		ResourcePrefix:  strings.ToLower(env(s.cfg)) + "-",
		LatestTimestamp: &stats.LatestTimestamp,
		Counts:          counts,
	}
	return renderPage(ctx, w, page, s.templates[versionsTemplate])
}

func env(cfg *config.Config) string {
	e := cfg.DeploymentEnvironment()
	return strings.ToUpper(e[:1]) + e[1:]
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
