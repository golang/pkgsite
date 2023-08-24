// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config provides the definition of the configuration for the
// frontend.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// AppVersionFormat is the expected format of the app version timestamp.
const AppVersionFormat = "20060102t150405"

const (
	// BypassQuotaAuthHeader is the header key used by the frontend server to know
	// that a request can bypass the quota server.
	BypassQuotaAuthHeader = "X-Go-Discovery-Auth-Bypass-Quota"

	// BypassCacheAuthHeader is the header key used by the frontend server to
	// know that a request can bypass cache.
	BypassCacheAuthHeader = "X-Go-Discovery-Auth-Bypass-Cache"

	// BypassErrorReportingHeader is the header key used by the ErrorReporting middleware
	// to avoid calling the errorreporting service.
	BypassErrorReportingHeader = "X-Go-Discovery-Bypass-Error-Reporting"

	// AllowDebugHeader is the header key used by the frontend server that allows
	// serving debug pages.
	AllowDebugHeader = "X-Go-Discovery-Debug"
)

// Config holds shared configuration values used in instantiating our server
// components.
type Config struct {
	// AuthValues is the set of values that could be set on the AuthHeader, in
	// order to bypass checks by the cache.
	AuthValues []string

	// Discovery environment variables
	ProxyURL, IndexURL string

	// Ports used for hosting. 'DebugPort' is used for serving HTTP debug pages.
	Port, DebugPort string

	// AppEngine identifiers
	ProjectID, ServiceID, VersionID, ZoneID, InstanceID, LocationID string

	// ServiceAccount is the email of the service account that this process
	// is running as when on GCP.
	ServiceAccount string

	// QueueURL is the URL that the Cloud Tasks queue should send requests to.
	// It should be used when the worker is not on AppEngine.
	QueueURL string

	// QueueAudience is used to allow the Cloud Tasks queue to authorize itself
	// to the worker. It should be the OAuth 2.0 client ID associated with the
	// IAP that is gating access to the worker.
	QueueAudience string

	// GoogleTagManagerID is the ID used for GoogleTagManager. It has the
	// structure GTM-XXXX.
	GoogleTagManagerID string

	// MonitoredResource represents the resource that is running the current binary.
	// It might be a Google AppEngine app, a Cloud Run service, or a Kubernetes pod.
	// See https://cloud.google.com/monitoring/api/resources for more
	// details:
	// "An object representing a resource that can be used for monitoring, logging,
	// billing, or other purposes. Examples include virtual machine instances,
	// databases, and storage devices such as disks.""
	MonitoredResource *MonitoredResource

	// FallbackVersionLabel is used as the VersionLabel when not hosting on
	// AppEngine.
	FallbackVersionLabel string

	DBSecret, DBUser, DBHost, DBPort, DBName, DBSSL string
	DBSecondaryHost                                 string // DB host to use if first one is down
	DBPassword                                      string `json:"-" yaml:"-"`

	// Configuration for redis page cache.
	RedisCacheHost, RedisBetaCacheHost, RedisCachePort string

	// UseProfiler specifies whether to enable Stackdriver Profiler.
	UseProfiler bool

	Quota QuotaSettings

	// Minimum log level below which no logs will be printed.
	// Possible values are [debug, info, error, fatal].
	// In case of invalid/empty value, all logs will be printed.
	LogLevel string

	// DynamicConfigLocation is the location (either a file or gs://bucket/object) for
	// dynamic configuration.
	DynamicConfigLocation string

	// DynamicExcludeLocation is the location (either a file or gs://bucket/object) for
	// dynamic exclusion file.
	DynamicExcludeLocation string

	// ServeStats determines whether the server has an endpoint that serves statistics for
	// benchmarking or other purposes.
	ServeStats bool

	// DisableErrorReporting disables sending errors to the GCP ErrorReporting system.
	DisableErrorReporting bool

	// VulnDB is the URL of the Go vulnerability DB.
	VulnDB string
}

// MonitoredResource represents the resource that is running the current binary.
// It might be a Google AppEngine app, a Cloud Run service, or a Kubernetes pod.
// See https://cloud.google.com/monitoring/api/resources for more
// details:
// "An object representing a resource that can be used for monitoring, logging,
// billing, or other purposes. Examples include virtual machine instances,
// databases, and storage devices such as disks."
type MonitoredResource struct {
	Type string `yaml:"type,omitempty"`

	Labels map[string]string `yaml:"labels,omitempty"`
}

// AppVersionLabel returns the version label for the current instance.  This is
// the AppVersionID available, otherwise a string constructed using the
// timestamp of process start.
func (c *Config) AppVersionLabel() string {
	if c.VersionID != "" {
		return c.VersionID
	}
	return c.FallbackVersionLabel
}

// StatementTimeout is the value of the Postgres statement_timeout parameter.
// Statements that run longer than this are terminated.
// 10 minutes is the App Engine standard request timeout,
// but we set this longer for the worker.
const StatementTimeout = 30 * time.Minute

// SourceTimeout is the value of the timeout for source.Client, which is used
// to fetch source code from third party URLs.
const SourceTimeout = 1 * time.Minute

// TaskIDChangeIntervalFrontend is the time period during which a given module
// version can be re-enqueued to frontend tasks.
const TaskIDChangeIntervalFrontend = 30 * time.Minute

// DBConnInfo returns a PostgreSQL connection string constructed from
// environment variables, using the primary database host.
func (c *Config) DBConnInfo() string {
	return c.dbConnInfo(c.DBHost)
}

// DBSecondaryConnInfo returns a PostgreSQL connection string constructed from
// environment variables, using the backup database host. It returns the
// empty string if no backup is configured.
func (c *Config) DBSecondaryConnInfo() string {
	if c.DBSecondaryHost == "" {
		return ""
	}
	return c.dbConnInfo(c.DBSecondaryHost)
}

// dbConnInfo returns a PostgresSQL connection string for the given host.
func (c *Config) dbConnInfo(host string) string {
	// For the connection string syntax, see
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING.
	// Set the statement_timeout config parameter for this session.
	// See https://www.postgresql.org/docs/current/runtime-config-client.html.
	timeoutOption := fmt.Sprintf("-c statement_timeout=%d", StatementTimeout/time.Millisecond)
	return fmt.Sprintf(
		"user='%s' password='%s' host='%s' port=%s dbname='%s' sslmode='%s' options='%s'",
		c.DBUser, c.DBPassword, host, c.DBPort, c.DBName, c.DBSSL, timeoutOption,
	)
}

// HostAddr returns the network on which to serve the primary HTTP service.
func (c *Config) HostAddr(dflt string) string {
	if c.Port != "" {
		return fmt.Sprintf(":%s", c.Port)
	}
	return dflt
}

// DebugAddr returns the network address on which to serve debugging
// information.
func (c *Config) DebugAddr(dflt string) string {
	if c.DebugPort != "" {
		return fmt.Sprintf(":%s", c.DebugPort)
	}
	return dflt
}

// DeploymentEnvironment returns the deployment environment this process
// is in: usually one of "local", "exp", "dev", "staging" or "prod".
func (c *Config) DeploymentEnvironment() string {
	if c.ServiceID == "" {
		return "local"
	}
	before, _, found := strings.Cut(c.ServiceID, "-")
	if !found {
		return "prod"
	}
	if before == "" {
		return "unknownEnv"
	}
	return before
}

// Application returns the name of the running application: "worker",
// "frontend", etc.
func (c *Config) Application() string {
	if c.ServiceID == "" {
		return "unknownApp"
	}
	before, after, found := strings.Cut(c.ServiceID, "-")
	var svc string
	if !found {
		svc = before
	} else {
		svc = after
	}
	switch svc {
	case "default":
		return "frontend"
	default:
		return svc
	}
}

// QuotaSettings is config for internal/middleware/quota.go
type QuotaSettings struct {
	Enable     bool `yaml:"Enable"`
	QPS        int  `yaml:"QPS"`        // allowed queries per second, per IP block
	Burst      int  `yaml:"Burst"`      // maximum requests per second, per block; the size of the token bucket
	MaxEntries int  `yaml:"MaxEntries"` // maximum number of entries to keep track of
	// Record data about blocking, but do not actually block.
	// This is a *bool, so we can distinguish "not present" from "false" in an override
	RecordOnly *bool `yaml:"RecordOnly"`
	// AuthValues is the set of values that could be set on the AuthHeader, in
	// order to bypass checks by the quota server.
	AuthValues []string `yaml:"AuthValues"`
	HMACKey    []byte   `json:"-" yaml:"-"` // key for obfuscating IPs
}

// Dump outputs the current config information to the given Writer.
func (c *Config) Dump(w io.Writer) error {
	fmt.Fprint(w, "config: ")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(c)
}
