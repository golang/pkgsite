// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
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
		return err
	}
	pms, err := getProcessMemStats()
	if err != nil {
		return err
	}

	page := struct {
		Config          *config.Config
		Env             string
		ResourcePrefix  string
		LatestTimestamp *time.Time
		LocationID      string
		Experiments     []*internal.Experiment
		Excluded        []string
		LoadShedStats   fetch.LoadShedStats
		GoMemStats      runtime.MemStats
		ProcessStats    processMemStats
		SystemStats     systemMemStats
	}{
		Config:         s.cfg,
		Env:            env(s.cfg),
		ResourcePrefix: strings.ToLower(env(s.cfg)) + "-",
		LocationID:     s.cfg.LocationID,
		Experiments:    experiments,
		Excluded:       excluded,
		LoadShedStats:  fetch.ZipLoadShedStats(),
		GoMemStats:     gms,
		ProcessStats:   pms,
		SystemStats:    sms,
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

// systemMemStats holds values from the /proc/meminfo
// file, which describes the total system memory.
// All values are in bytes.
type systemMemStats struct {
	Total     uint64
	Free      uint64
	Available uint64
	Used      uint64
	Buffers   uint64
	Cached    uint64
}

// getSystemMemStats reads the /proc/meminfo file.
func getSystemMemStats() (systemMemStats, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return systemMemStats{}, err
	}
	defer f.Close()

	// Read the number, convert kibibytes to bytes, and set p.
	set := func(p *uint64, words []string) {
		if len(words) != 3 || words[2] != "kB" {
			err = fmt.Errorf("got %+v, want 3 words, third is 'kB'", words)
			return
		}
		var ki uint64
		ki, err = strconv.ParseUint(words[1], 10, 64)
		if err == nil {
			*p = ki * 1024
		}
	}

	scan := bufio.NewScanner(f)
	var sms systemMemStats
	for scan.Scan() {
		words := strings.Fields(scan.Text())
		switch words[0] {
		case "MemTotal:":
			set(&sms.Total, words)
		case "MemFree:":
			set(&sms.Free, words)
		case "MemAvailable:":
			set(&sms.Available, words)
		case "Buffers:":
			set(&sms.Buffers, words)
		case "Cached:":
			set(&sms.Cached, words)
		}
	}
	if err != nil {
		return systemMemStats{}, err
	}
	sms.Used = sms.Total - sms.Free - sms.Buffers - sms.Cached // see `man free`
	return sms, nil
}

// processMemStats holds values that describe the current process's memory.
// All values are in bytes.
type processMemStats struct {
	VSize uint64 // virtual memory size
	RSS   uint64 // resident set size (physical memory in use)
}

func getProcessMemStats() (processMemStats, error) {
	f, err := os.Open("/proc/self/stat")
	if err != nil {
		return processMemStats{}, err
	}
	defer f.Close()
	// Values from `man proc`.
	var (
		d          int
		s          string
		c          byte
		vsize, rss uint64
	)
	_, err = fmt.Fscanf(f, "%d %s %c %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d %d",
		&d, &s, &c, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &d, &vsize, &rss)
	if err != nil {
		return processMemStats{}, err
	}
	const pageSize = 4 * 1024 // Linux page size, from `getconf PAGESIZE`
	return processMemStats{
		VSize: vsize,
		RSS:   rss * pageSize,
	}, nil
}
