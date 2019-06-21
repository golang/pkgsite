// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config provides facilities for resolving discovery configuration
// parameters from the hosting environment.
// TODO(rfindley): factor out more functionality here.
package config

import (
	"fmt"
	"os"
)

// DebugAddr returns the network address on which to serve debugging
// information.
func DebugAddr(dflt string) string {
	if port := os.Getenv("DEBUG_PORT"); port != "" {
		return fmt.Sprintf(":%s", port)
	}
	return dflt
}
