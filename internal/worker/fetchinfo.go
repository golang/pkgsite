// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"sort"
	"sync"
	"time"
)

// FetchInfo describes a fetch in progress, or completed.
// It is used to display information on the worker home page.
type FetchInfo struct {
	ModulePath string
	Version    string
	ZipSize    uint64
	Start      time.Time
	Finish     time.Time
	Status     int
	Error      error
}

var (
	fetchInfoMu  sync.Mutex
	fetchInfoMap = map[*FetchInfo]struct{}{}
)

func init() {
	// Start a goroutine to remove FetchInfos that have been finished for a
	// while.
	const linger = time.Minute
	go func() {
		for {
			now := time.Now()
			fetchInfoMu.Lock()
			for fi := range fetchInfoMap {
				if !fi.Finish.IsZero() && now.Sub(fi.Finish) > linger {
					delete(fetchInfoMap, fi)
				}
			}
			fetchInfoMu.Unlock()
			time.Sleep(linger)
		}
	}()
}

func startFetchInfo(fi *FetchInfo) {
	fetchInfoMu.Lock()
	defer fetchInfoMu.Unlock()
	fetchInfoMap[fi] = struct{}{}
}

func finishFetchInfo(fi *FetchInfo, status int, err error) {
	fetchInfoMu.Lock()
	defer fetchInfoMu.Unlock()
	fi.Finish = time.Now()
	fi.Status = status
	fi.Error = err
}

// FetchInfos returns information about all fetches in progress,
// sorted by start time.
func FetchInfos() []*FetchInfo {
	var fis []*FetchInfo
	fetchInfoMu.Lock()
	for fi := range fetchInfoMap {
		// Copy to avoid races on Status and Error when read by
		// worker home page.
		cfi := *fi
		fis = append(fis, &cfi)
	}
	fetchInfoMu.Unlock()
	// Order first by done-ness, then by age.
	sort.Slice(fis, func(i, j int) bool {
		if (fis[i].Status == 0) == (fis[j].Status == 0) {
			return fis[i].Start.Before(fis[j].Start)
		}
		return fis[i].Status == 0
	})
	return fis
}
