// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package config resolves shared configuration for Go Discovery services, and
// provides functions to access this configuration.
//
// The Init function should be called before using any of the configuration accessors.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"golang.org/x/discovery/internal/secrets"
	"golang.org/x/net/context/ctxhttp"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// HostAddr returns the network on which to serve the primary HTTP service.
func HostAddr(dflt string) string {
	if cfg.Port != "" {
		return fmt.Sprintf(":%s", cfg.Port)
	}
	return dflt
}

// DebugAddr returns the network address on which to serve debugging
// information.
func DebugAddr(dflt string) string {
	if cfg.DebugPort != "" {
		return fmt.Sprintf(":%s", cfg.DebugPort)
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
	return cfg.IndexURL
}

// ProxyURL returns the URL of the Go module proxy.
func ProxyURL() string {
	return cfg.ProxyURL
}

// ServiceID returns a the name of the current application.
func ServiceID() string {
	return cfg.ServiceID
}

// InstanceID returns a unique identifier for this process instance.
func InstanceID() string {
	return cfg.InstanceID
}

// LocationID returns the region containing our AppEngine apps.
func LocationID() string {
	return cfg.LocationID
}

// AppVersionLabel returns the version label for the current instance.  This is
// the AppVersionID available, otherwise a string constructed using the
// timestamp of process start.
func AppVersionLabel() string {
	if cfg.VersionID != "" {
		return cfg.VersionID
	}
	return cfg.FallbackVersionLabel
}

// AppVersionID is the AppEngine version of the current instance.
func AppVersionID() string {
	return cfg.VersionID
}

// AppVersionFormat is the expected format of the app version timestamp.
const AppVersionFormat = "20060102t150405"

// ValidateAppVersion validates that appVersion follows the expected format
// defined by AppVersionFormat.
func ValidateAppVersion(appVersion string) error {
	if _, err := time.Parse(AppVersionFormat, appVersion); err != nil {
		return fmt.Errorf("time.Parse(%q, %q): %v", AppVersionFormat, appVersion, err)
	}
	return nil
}

// ProjectID returns the GCP project ID.
func ProjectID() string {
	return cfg.ProjectID
}

// ZoneID returns the GAE zone.
func ZoneID() string {
	return cfg.ZoneID
}

// AppMonitoredResource is the resource for the current GAE app.
// See https://cloud.google.com/monitoring/api/resources#tag_gae_app for more
// details:
// "An object representing a resource that can be used for monitoring, logging,
// billing, or other purposes. Examples include virtual machine instances,
// databases, and storage devices such as disks.""
func AppMonitoredResource() *mrpb.MonitoredResource {
	return cfg.AppMonitoredResource
}

// OnAppEngine reports if the current process is running in an AppEngine
// environment.
func OnAppEngine() bool {
	// TODO(rfindley): verify that this works for the go1.12 runtime
	return cfg.GaeEnv == "standard"
}

// DBConnInfo returns a PostgreSQL connection string constructed from
// environment variables.
func DBConnInfo() string {
	return fmt.Sprintf("user='%s' password='%s' host='%s' dbname='%s' sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBName)
}

type config struct {
	// Discovery environment variables
	ProxyURL, IndexURL string

	// Ports used for hosting. 'DebugPort' is used for serving HTTP debug pages.
	Port, DebugPort string

	// AppEngine identifiers
	ProjectID, ServiceID, VersionID, ZoneID, InstanceID, LocationID string

	GaeEnv string

	// StackDriver resource identifiers
	AppMonitoredResource *mrpb.MonitoredResource

	// FallbackVersionLabel is used as the VersionLabel when not hosting on
	// AppEngine.
	FallbackVersionLabel string

	DBSecret, DBUser, DBHost, DBName string

	DBPassword string `json:"-"`
}

var cfg config

// Init resolves all configuration values provided by the config package. It
// must be called before any configuration values are used.
func Init(ctx context.Context) error {
	// Resolve client/server configuration from the environment.
	cfg.IndexURL = GetEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index")
	cfg.ProxyURL = GetEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org")
	cfg.Port = os.Getenv("PORT")
	cfg.DebugPort = os.Getenv("DEBUG_PORT")

	// Resolve AppEngine identifiers
	cfg.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	cfg.ServiceID = os.Getenv("GAE_SERVICE")
	cfg.VersionID = os.Getenv("GAE_VERSION")
	cfg.InstanceID = os.Getenv("GAE_INSTANCE")
	cfg.GaeEnv = os.Getenv("GAE_ENV")

	// locationID is essentially hard-coded until we figure out a good way to
	// determine it programmatically, but we check an environment variable in
	// case it needs to be overridden.
	cfg.LocationID = GetEnv("GO_DISCOVERY_GAE_LOCATION_ID", "us-central1")

	if cfg.GaeEnv != "" {
		// zone is not available in the environment but can be queried via the
		// metadata api as described at
		// https://cloud.google.com/appengine/docs/standard/java/accessing-instance-metadata
		// (this documentation doesn't exist for Golang, but it seems to work).
		zoneURL := "http://metadata.google.internal/computeMetadata/v1/instance/zone"
		req, err := http.NewRequest("GET", zoneURL, nil)
		if err != nil {
			return fmt.Errorf("error creating metadata client: %v", err)
		}
		req.Header.Set("Metadata-Flavor", "Google")
		resp, err := ctxhttp.Do(ctx, nil, req)
		if err != nil {
			return fmt.Errorf("error resolving zone metadata: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("got status code %d when querying metadata", http.StatusOK)
		}
		defer resp.Body.Close()
		zoneBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading zone query body: %v", err)
		}
		cfg.ZoneID = path.Base(string(zoneBytes))
	}

	// this fallback should only be used when developing locally.
	cfg.FallbackVersionLabel = time.Now().Format(AppVersionFormat)

	cfg.AppMonitoredResource = &mrpb.MonitoredResource{
		Type: "gae_app",
		Labels: map[string]string{
			"project_id": cfg.ProjectID,
			"module_id":  cfg.ServiceID,
			"version_id": cfg.VersionID,
			"zone_id":    cfg.ZoneID,
		},
	}

	cfg.DBUser = GetEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	cfg.DBPassword = os.Getenv("GO_DISCOVERY_DATABASE_PASSWORD")
	cfg.DBHost = GetEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	cfg.DBName = GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	cfg.DBSecret = os.Getenv("GO_DISCOVERY_DATABASE_SECRET")

	if cfg.DBSecret != "" {
		var err error
		cfg.DBPassword, err = secrets.Get(ctx, cfg.DBSecret)
		if err != nil {
			return fmt.Errorf("could not get database password secret: %v", err)
		}
	}
	return nil
}

// Dump outputs the current config information to the given Writer.
func Dump(w io.Writer) error {
	fmt.Fprint(w, "config: ")
	return json.NewEncoder(w).Encode(cfg)
}
