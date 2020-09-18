// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"math"
	"os"
	"strconv"
	"sync"

	"golang.org/x/pkgsite/internal/log"
)

var (
	// The maximum size of zips being processed. If an incoming module would
	// cause zipSizeInFlight to exceed this value, it won't be processed.
	maxZipSizeInFlight uint64 = math.MaxUint64

	// Protects the variables below, and also serializes shedding decisions so
	// multiple simultaneous requests are handled properly.
	shedmu sync.Mutex

	// The total size of all zips currently being processed. We treat zip size
	// as a proxy for the total memory consumed by processing a module, and use
	// it to decide whether we can currently afford to process a module.
	zipSizeInFlight uint64

	fetchesInFlight int // number of fetches currently in progress
	totalRequests   int // total fetch requests
	sheddedRequests int // number of requests that were shedded

)

func init() {
	ctx := context.Background()
	m := os.Getenv("GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI")
	if m != "" {
		mebis, err := strconv.ParseUint(m, 10, 64)
		if err != nil {
			log.Errorf(ctx, "could not parse GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI value %q", m)
		} else if mebis == 0 {
			log.Errorf(ctx, "bad value for GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI: %d. Must be >= 1.", mebis)
		} else {
			log.Infof(ctx, "shedding load over %dMi", mebis)
			maxZipSizeInFlight = mebis * mib
		}
	}
}

// decideToShed reports whether a module whose zip file is zipSize bytes should
// be shed (not processed). Its second return value is a function that should be
// deferred by the caller.
func decideToShed(zipSize uint64) (shouldShed bool, deferFunc func()) {
	shedmu.Lock()
	defer shedmu.Unlock()

	totalRequests++
	if zipSizeInFlight+zipSize > maxZipSizeInFlight {
		sheddedRequests++
		return true, func() {}
	}
	zipSizeInFlight += zipSize
	fetchesInFlight++
	return false, func() {
		shedmu.Lock()
		defer shedmu.Unlock()
		zipSizeInFlight -= zipSize
		fetchesInFlight--
	}
}

type loadShedStats struct {
	FetchesInFlight     int
	ZipBytesInFlight    uint64
	MaxZipBytesInFlight uint64
	SheddedRequests     int
	TotalRequests       int
}

func getLoadShedStats() loadShedStats {
	shedmu.Lock()
	defer shedmu.Unlock()
	return loadShedStats{
		FetchesInFlight:     fetchesInFlight,
		ZipBytesInFlight:    zipSizeInFlight,
		MaxZipBytesInFlight: maxZipSizeInFlight,
		SheddedRequests:     sheddedRequests,
		TotalRequests:       totalRequests,
	}
}
