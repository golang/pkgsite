// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config provides facilities for resolving discovery configuration
// parameters from the hosting environment.
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/discovery/internal/secrets"
)

// HostAddr returns the network on which to serve the primary HTTP service.
func HostAddr(dflt string) string {
	if port := os.Getenv("PORT"); port != "" {
		return fmt.Sprintf(":%s", port)
	}
	return dflt
}

// DebugAddr returns the network address on which to serve debugging
// information.
func DebugAddr(dflt string) string {
	if port := os.Getenv("DEBUG_PORT"); port != "" {
		return fmt.Sprintf(":%s", port)
	}
	return dflt
}

// GetEnv looks up the given key from the environment, returning its value if
// it exists, and otherwise returning the given fallback value.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// IndexURL returns the URL of the Go module index.
func IndexURL() string {
	return GetEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index")
}

// ProxyURL returns the URL of the Go module proxy.
func ProxyURL() string {
	return GetEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org")
}

// ServiceID returns a the name of the current application.
func ServiceID() string {
	if app := os.Getenv("GAE_SERVICE"); app != "" {
		return app
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Base(exe)
	}
	return ""
}

// InstanceID returns a unique identifier for this process instance.
func InstanceID() string {
	return os.Getenv("GAE_INSTANCE")
}

// AppVersionLabel returns the version label for the current instance.  This is
// the AppEngine version if available, otherwise a string constructed using the
// timestamp of process start.
func AppVersionLabel() string {
	if gv := os.Getenv("GAE_VERSION"); gv != "" {
		return gv
	}
	return fallbackVersionLabel
}

var fallbackVersionLabel string

func init() {
	fallbackVersionLabel = time.Now().Format("20060102t150405")
}

// OnAppEngine reports if the current process is running in an AppEngine
// environment.
func OnAppEngine() bool {
	// TODO(rfindley): verify that this works for the go1.12 runtime
	return os.Getenv("GAE_ENV") == "standard"
}

// DBConnInfo returns a PostgreSQL connection string constructed from
// environment variables.
func DBConnInfo(ctx context.Context, secret string) (string, error) {
	var (
		user     = GetEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
		password = GetEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
		host     = GetEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
		dbname   = GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	)

	// When running on App Engine, the runtime sets GAE_ENV to 'standard' per
	// https://cloud.google.com/appengine/docs/standard/go111/runtime
	if OnAppEngine() {
		var err error
		password, err = secrets.Get(ctx, secret)
		if err != nil {
			return "", fmt.Errorf("could not get database password secret: %v", err)
		}
	}
	return fmt.Sprintf("user='%s' password='%s' host='%s' dbname='%s' sslmode=disable", user, password, host, dbname), nil
}
