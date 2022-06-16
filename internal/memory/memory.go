// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package memory provides functions to collect memory information from a
// variety of places.
package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ReadRuntimeStats is a convenience for runtime.ReadMemStats.
func ReadRuntimeStats() runtime.MemStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

// SystemStats holds values from the /proc/meminfo
// file, which describes the total system memory.
// All values are in bytes.
type SystemStats struct {
	Total     uint64
	Free      uint64
	Available uint64
	Used      uint64
	Buffers   uint64
	Cached    uint64
}

// ReadSystemStats reads the /proc/meminfo file to get information about the
// machine.
func ReadSystemStats() (SystemStats, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return SystemStats{}, err
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
	var sms SystemStats
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
		return SystemStats{}, err
	}
	sms.Used = sms.Total - sms.Free - sms.Buffers - sms.Cached // see `man free`
	return sms, nil
}

// ProcessStats holds values that describe the current process's memory.
// All values are in bytes.
type ProcessStats struct {
	VSize uint64 // virtual memory size
	RSS   uint64 // resident set size (physical memory in use)
}

// ReadProcessStats reads memory stats for the process.
func ReadProcessStats() (ProcessStats, error) {
	f, err := os.Open("/proc/self/stat")
	if err != nil {
		return ProcessStats{}, err
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
		return ProcessStats{}, err
	}
	const pageSize = 4 * 1024 // Linux page size, from `getconf PAGESIZE`
	return ProcessStats{
		VSize: vsize,
		RSS:   rss * pageSize,
	}, nil
}

// ReadCgroupStats reads memory information for the current cgroup.
// (A cgroup is the sandbox in which a docker container runs.)
// All values are in bytes.
func ReadCgroupStats() (map[string]uint64, error) {
	m, err := getCgroupStats()
	if err != nil {
		return nil, err
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
	// True RSS. See note on https://lwn.net/Articles/432224.
	m["trueRSS"] = m["rss"] + m["mapped_file"]
	return m, nil
}

func getCgroupStats() (map[string]uint64, error) {
	const cgroupMemDir = "/sys/fs/cgroup/memory"

	readUintFile := func(filename string) (uint64, error) {
		data, err := os.ReadFile(filepath.Join(cgroupMemDir, filename))
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

// Format formats a memory value for humans. It uses a B, K, M or G suffix and
// rounds to two decimal places.
func Format(m uint64) string {
	const Ki = 1024

	if m < Ki {
		return fmt.Sprintf("%d B", m)
	}
	if m < Ki*Ki {
		return fmt.Sprintf("%.2f K", float64(m)/Ki)
	}
	if m < Ki*Ki*Ki {
		return fmt.Sprintf("%.2f M", float64(m)/(Ki*Ki))
	}
	return fmt.Sprintf("%.2f G", float64(m)/(Ki*Ki*Ki))
}
