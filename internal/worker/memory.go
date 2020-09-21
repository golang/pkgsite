// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Functions to collect memory information from a variety of places.

package worker

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/pkgsite/internal/log"
)

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

// getSystemMemStats reads the /proc/meminfo file to get information about the
// machine.
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
	if err == nil {
		err = scan.Err()
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

// Read memory information for the current cgroup.
// (A cgroup is the sandbox in which a docker container runs.)
// All values are in bytes.
// Returns nil on any error.
func getCgroupMemStats() map[string]uint64 {
	m, err := getCgroupMemStatsErr()
	if err != nil {
		log.Warningf(context.Background(), "getCgroupMemStats: %v", err)
		return nil
	}
	// k8s's definition of container memory, as shown by `kubectl top pods`.
	// See https://www.magalix.com/blog/memory_working_set-vs-memory_rss.
	workingSet := m["usage"]
	tif := m["total_inactive_file"]
	if tif > workingSet {
		workingSet = 0
	} else {
		workingSet -= tif
	}
	m["workingSet"] = workingSet
	return m
}

func getCgroupMemStatsErr() (map[string]uint64, error) {
	const cgroupMemDir = "/sys/fs/cgroup/memory"

	readUintFile := func(filename string) (uint64, error) {
		data, err := ioutil.ReadFile(filepath.Join(cgroupMemDir, filename))
		if err != nil {
			return 0, err
		}
		u, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return 0, err
		}
		return u, nil
	}

	m := map[string]uint64{}
	var err error
	m["limit"], err = readUintFile("memory.limit_in_bytes")
	if err != nil {
		return nil, err
	}
	m["usage"], err = readUintFile("memory.usage_in_bytes")
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(cgroupMemDir, "memory.stat"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		fs := strings.Fields(scan.Text())
		if len(fs) != 2 {
			return nil, fmt.Errorf("memory.stat: %q: not two fields", scan.Text())
		}
		u, err := strconv.ParseUint(fs[1], 10, 64)
		if err != nil {
			return nil, err
		}
		m[fs[0]] = u
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return m, nil
}
