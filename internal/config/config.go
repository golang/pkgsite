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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ghodss/yaml"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/secrets"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// GetEnv looks up the given key from the environment, returning its value if
// it exists, and otherwise returning the given fallback value.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetEnvInt looks up the given key from the environment and expects an integer,
// returning the integer value if it exists, and otherwise returning the given
// fallback value.
// If the environment variable has a value but it can't be parsed as an integer,
// GetEnvInt terminates the program.
func GetEnvInt(ctx context.Context, key string, fallback int) int {
	if s, ok := os.LookupEnv(key); ok {
		v, err := strconv.Atoi(s)
		if err != nil {
			log.Fatalf(ctx, "bad value %q for %s: %v", s, key, err)
		}
		return v
	}
	return fallback
}

// GetEnvFloat64 looks up the given key from the environment and expects a
// float64, returning the float64 value if it exists, and otherwise returning
// the given fallback value.
func GetEnvFloat64(key string, fallback float64) float64 {
	if valueStr, ok := os.LookupEnv(key); ok {
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			return value
		}
	}
	return fallback
}

// AppVersionFormat is the expected format of the app version timestamp.
const AppVersionFormat = "20060102t150405"

// ValidateAppVersion validates that appVersion follows the expected format
// defined by AppVersionFormat.
func ValidateAppVersion(appVersion string) error {
	// Accept GKE versions, which start with the docker image name.
	if strings.HasPrefix(appVersion, "gcr.io/") {
		return nil
	}
	if _, err := time.Parse(AppVersionFormat, appVersion); err != nil {
		// Accept alternative version, used by our AppEngine deployment script.
		const altDateFormat = "2006-01-02t15-04"
		if len(appVersion) > len(altDateFormat) {
			appVersion = appVersion[:len(altDateFormat)]
		}
		if _, err := time.Parse(altDateFormat, appVersion); err != nil {
			return fmt.Errorf("app version %q does not match time formats %q or %q: %v",
				appVersion, AppVersionFormat, altDateFormat, err)
		}
	}
	return nil
}

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
	// It might be a Google AppEngine app or a Kubernetes pod.
	// See https://cloud.google.com/monitoring/api/resources for more
	// details:
	// "An object representing a resource that can be used for monitoring, logging,
	// billing, or other purposes. Examples include virtual machine instances,
	// databases, and storage devices such as disks.""
	MonitoredResource *mrpb.MonitoredResource

	// FallbackVersionLabel is used as the VersionLabel when not hosting on
	// AppEngine.
	FallbackVersionLabel string

	DBSecret, DBUser, DBHost, DBPort, DBName string
	DBSecondaryHost                          string // DB host to use if first one is down
	DBPassword                               string `json:"-"`

	// Configuration for redis page cache.
	RedisCacheHost, RedisCachePort string

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

	// ServeStats determines whether the server has an endpoint that serves statistics for
	// benchmarking or other purposes.
	ServeStats bool

	// DisableErrorReporting disables sending errors to the GCP ErrorReporting system.
	DisableErrorReporting bool

	// VulnDB is the URL of the Go vulnerability DB.
	VulnDB string
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

// OnAppEngine reports if the current process is running in an AppEngine
// environment.
func (c *Config) OnAppEngine() bool {
	return os.Getenv("GAE_ENV") == "standard"
}

// OnGKE reports whether the current process is running on GKE.
func (c *Config) OnGKE() bool {
	return os.Getenv("GO_DISCOVERY_ON_GKE") == "true"
}

// OnGCP reports whether the current process is running on Google Cloud
// Platform.
func (c *Config) OnGCP() bool {
	return c.OnAppEngine() || c.OnGKE()
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
	return fmt.Sprintf("user='%s' password='%s' host='%s' port=%s dbname='%s' sslmode=disable options='%s'",
		c.DBUser, c.DBPassword, host, c.DBPort, c.DBName, timeoutOption)
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
	parts := strings.SplitN(c.ServiceID, "-", 2)
	if len(parts) == 1 {
		return "prod"
	}
	if parts[0] == "" {
		return "unknownEnv"
	}
	return parts[0]
}

// Application returns the name of the running application: "worker",
// "frontend", etc.
func (c *Config) Application() string {
	if c.ServiceID == "" {
		return "unknownApp"
	}
	parts := strings.SplitN(c.ServiceID, "-", 2)
	var svc string
	if len(parts) == 1 {
		svc = parts[0]
	} else {
		svc = parts[1]
	}
	switch svc {
	case "default":
		return "frontend"
	default:
		return svc
	}
}

// configOverride holds selected config settings that can be dynamically overridden.
type configOverride struct {
	DBHost          string
	DBSecondaryHost string
	DBName          string
	Quota           QuotaSettings
}

// QuotaSettings is config for internal/middleware/quota.go
type QuotaSettings struct {
	Enable     bool
	QPS        int // allowed queries per second, per IP block
	Burst      int // maximum requests per second, per block; the size of the token bucket
	MaxEntries int // maximum number of entries to keep track of
	// Record data about blocking, but do not actually block.
	// This is a *bool, so we can distinguish "not present" from "false" in an override
	RecordOnly *bool
	// AuthValues is the set of values that could be set on the AuthHeader, in
	// order to bypass checks by the quota server.
	AuthValues []string
	HMACKey    []byte `json:"-"` // key for obfuscating IPs
}

// Init resolves all configuration values provided by the config package. It
// must be called before any configuration values are used.
func Init(ctx context.Context) (_ *Config, err error) {
	defer derrors.Add(&err, "config.Init(ctx)")
	// Build a Config from the execution environment, loading some values
	// from envvars and others from remote services.
	cfg := &Config{
		AuthValues: parseCommaList(os.Getenv("GO_DISCOVERY_AUTH_VALUES")),
		IndexURL:   GetEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index"),
		ProxyURL:   GetEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org"),
		Port:       os.Getenv("PORT"),
		DebugPort:  os.Getenv("DEBUG_PORT"),
		// Resolve AppEngine identifiers
		ProjectID:          os.Getenv("GOOGLE_CLOUD_PROJECT"),
		ServiceID:          GetEnv("GAE_SERVICE", os.Getenv("GO_DISCOVERY_SERVICE")),
		VersionID:          GetEnv("GAE_VERSION", os.Getenv("DOCKER_IMAGE")),
		InstanceID:         GetEnv("GAE_INSTANCE", os.Getenv("GO_DISCOVERY_INSTANCE")),
		GoogleTagManagerID: os.Getenv("GO_DISCOVERY_GOOGLE_TAG_MANAGER_ID"),
		QueueURL:           os.Getenv("GO_DISCOVERY_QUEUE_URL"),
		QueueAudience:      os.Getenv("GO_DISCOVERY_QUEUE_AUDIENCE"),

		// LocationID is essentially hard-coded until we figure out a good way to
		// determine it programmatically, but we check an environment variable in
		// case it needs to be overridden.
		LocationID: GetEnv("GO_DISCOVERY_GAE_LOCATION_ID", "us-central1"),
		// This fallback should only be used when developing locally.
		FallbackVersionLabel: time.Now().Format(AppVersionFormat),
		DBHost:               chooseOne(GetEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")),
		DBUser:               GetEnv("GO_DISCOVERY_DATABASE_USER", "postgres"),
		DBPassword:           os.Getenv("GO_DISCOVERY_DATABASE_PASSWORD"),
		DBSecondaryHost:      chooseOne(os.Getenv("GO_DISCOVERY_DATABASE_SECONDARY_HOST")),
		DBPort:               GetEnv("GO_DISCOVERY_DATABASE_PORT", "5432"),
		DBName:               GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-db"),
		DBSecret:             os.Getenv("GO_DISCOVERY_DATABASE_SECRET"),
		RedisCacheHost:       os.Getenv("GO_DISCOVERY_REDIS_HOST"),
		RedisCachePort:       GetEnv("GO_DISCOVERY_REDIS_PORT", "6379"),
		Quota: QuotaSettings{
			Enable:     os.Getenv("GO_DISCOVERY_ENABLE_QUOTA") == "true",
			QPS:        GetEnvInt(ctx, "GO_DISCOVERY_QUOTA_QPS", 10),
			Burst:      20,   // ignored in redis-based quota implementation
			MaxEntries: 1000, // ignored in redis-based quota implementation
			RecordOnly: func() *bool {
				t := (os.Getenv("GO_DISCOVERY_QUOTA_RECORD_ONLY") != "false")
				return &t
			}(),
			AuthValues: parseCommaList(os.Getenv("GO_DISCOVERY_AUTH_VALUES")),
		},
		UseProfiler:           os.Getenv("GO_DISCOVERY_USE_PROFILER") == "true",
		LogLevel:              os.Getenv("GO_DISCOVERY_LOG_LEVEL"),
		ServeStats:            os.Getenv("GO_DISCOVERY_SERVE_STATS") == "true",
		DisableErrorReporting: os.Getenv("GO_DISCOVERY_DISABLE_ERROR_REPORTING") == "true",
		VulnDB:                GetEnv("GO_DISCOVERY_VULN_DB", "https://storage.googleapis.com/go-vulndb"),
	}
	log.SetLevel(cfg.LogLevel)

	bucket := os.Getenv("GO_DISCOVERY_CONFIG_BUCKET")
	object := os.Getenv("GO_DISCOVERY_CONFIG_DYNAMIC")
	if bucket != "" {
		if object == "" {
			return nil, errors.New("GO_DISCOVERY_CONFIG_DYNAMIC must be set if GO_DISCOVERY_CONFIG_BUCKET is")
		}
		cfg.DynamicConfigLocation = fmt.Sprintf("gs://%s/%s", bucket, object)
	} else {
		cfg.DynamicConfigLocation = object
	}
	if cfg.OnGCP() {
		// Zone is not available in the environment but can be queried via the metadata API.
		zone, err := gceMetadata(ctx, "instance/zone")
		if err != nil {
			return nil, err
		}
		cfg.ZoneID = zone
		sa, err := gceMetadata(ctx, "instance/service-accounts/default/email")
		if err != nil {
			return nil, err
		}
		cfg.ServiceAccount = sa
		switch {
		case cfg.OnAppEngine():
			// Use the gae_app monitored resource. It would be better to use the
			// gae_instance monitored resource, but that's not currently supported:
			// https://cloud.google.com/logging/docs/api/v2/resource-list#resource-types
			cfg.MonitoredResource = &mrpb.MonitoredResource{
				Type: "gae_app",
				Labels: map[string]string{
					"project_id": cfg.ProjectID,
					"module_id":  cfg.ServiceID,
					"version_id": cfg.VersionID,
					"zone":       cfg.ZoneID,
				},
			}
		case cfg.OnGKE():
			cfg.MonitoredResource = &mrpb.MonitoredResource{
				Type: "k8s_container",
				Labels: map[string]string{
					"project_id":     cfg.ProjectID,
					"location":       path.Base(cfg.ZoneID),
					"cluster_name":   cfg.DeploymentEnvironment() + "-pkgsite",
					"namespace_name": "default",
					"pod_name":       os.Getenv("HOSTNAME"),
					"container_name": cfg.Application(),
				},
			}
		default:
			return nil, errors.New("on GCP but using an unknown product")
		}
	} else { // running locally, perhaps
		cfg.MonitoredResource = &mrpb.MonitoredResource{
			Type:   "global",
			Labels: map[string]string{"project_id": cfg.ProjectID},
		}
	}
	if cfg.DBHost == "" {
		panic("DBHost is empty; impossible")
	}
	if cfg.DBSecret != "" {
		var err error
		cfg.DBPassword, err = secrets.Get(ctx, cfg.DBSecret)
		if err != nil {
			return nil, fmt.Errorf("could not get database password secret: %v", err)
		}
	}
	if cfg.Quota.Enable {
		s, err := secrets.Get(ctx, "quota-hmac-key")
		if err != nil {
			return nil, err
		}
		hmacKey, err := hex.DecodeString(s)
		if err != nil {
			return nil, err
		}
		if len(hmacKey) < 16 {
			return nil, errors.New("HMAC secret must be at least 16 bytes")
		}
		cfg.Quota.HMACKey = hmacKey
	} else {
		log.Debugf(ctx, "quota enforcement disabled")
	}

	// If the <env>-override.yaml file exists in the configured bucket, it
	// should provide overrides for selected configuration.
	// Use this when you want to fix something in prod quickly, without waiting
	// to re-deploy. (Otherwise, do not use it.)
	if cfg.DeploymentEnvironment() != "local" {
		overrideObj := fmt.Sprintf("%s-override.yaml", cfg.DeploymentEnvironment())
		overrideBytes, err := readOverrideFile(ctx, bucket, overrideObj)
		if err != nil {
			log.Error(ctx, err)
		} else {
			log.Infof(ctx, "processing overrides from gs://%s/%s", bucket, overrideObj)
			processOverrides(ctx, cfg, overrideBytes)
		}
	}
	return cfg, nil
}

func readOverrideFile(ctx context.Context, bucketName, objName string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "readOverrideFile(ctx, %q)", objName)

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	r, err := client.Bucket(bucketName).Object(objName).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ioutil.ReadAll(r)
}

func processOverrides(ctx context.Context, cfg *Config, bytes []byte) {
	var ov configOverride
	if err := yaml.Unmarshal(bytes, &ov); err != nil {
		log.Errorf(ctx, "processOverrides: yaml.Unmarshal: %v", err)
		return
	}
	overrideString(ctx, "DBHost", &cfg.DBHost, ov.DBHost)
	overrideString(ctx, "DBSecondaryHost", &cfg.DBSecondaryHost, ov.DBSecondaryHost)
	overrideString(ctx, "DBName", &cfg.DBName, ov.DBName)
	overrideInt(ctx, "Quota.QPS", &cfg.Quota.QPS, ov.Quota.QPS)
	overrideInt(ctx, "Quota.Burst", &cfg.Quota.Burst, ov.Quota.Burst)
	overrideInt(ctx, "Quota.MaxEntries", &cfg.Quota.MaxEntries, ov.Quota.MaxEntries)
	overrideBool(ctx, "Quota.RecordOnly", &cfg.Quota.RecordOnly, ov.Quota.RecordOnly)
}

func overrideString(ctx context.Context, name string, field *string, val string) {
	if val != "" {
		*field = val
		log.Infof(ctx, "overriding %s with %q", name, val)
	}
}

func overrideInt(ctx context.Context, name string, field *int, val int) {
	if val != 0 {
		*field = val
		log.Debugf(ctx, "overriding %s with %d", name, val)
	}
}

func overrideBool(ctx context.Context, name string, field **bool, val *bool) {
	if val != nil {
		*field = val
		log.Debugf(ctx, "overriding %s with %t", name, *val)
	}
}

// Dump outputs the current config information to the given Writer.
func (c *Config) Dump(w io.Writer) error {
	fmt.Fprint(w, "config: ")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(c)
}

// chooseOne selects one entry at random from a whitespace-separated
// string. It returns the empty string if there are no elements.
func chooseOne(configVar string) string {
	fields := strings.Fields(configVar)
	if len(fields) == 0 {
		return ""
	}
	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)
	return fields[rng.Intn(len(fields))]
}

// gceMetadata reads a metadata value from GCE.
// For the possible values of name, see
// https://cloud.google.com/appengine/docs/standard/java/accessing-instance-metadata.
func gceMetadata(ctx context.Context, name string) (_ string, err error) {
	// See https://cloud.google.com/appengine/docs/standard/java/accessing-instance-metadata.
	// (This documentation doesn't exist for Golang, but it seems to work).
	defer derrors.Wrap(&err, "gceMetadata(ctx, %q)", name)

	const metadataURL = "http://metadata.google.internal/computeMetadata/v1/"
	req, err := http.NewRequest("GET", metadataURL+name, nil)
	if err != nil {
		return "", fmt.Errorf("http.NewRequest: %v", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := ctxhttp.Do(ctx, nil, req)
	if err != nil {
		return "", fmt.Errorf("ctxhttp.Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ioutil.ReadAll: %v", err)
	}
	return string(bytes), nil
}

func parseCommaList(s string) []string {
	var a []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			a = append(a, p)
		}
	}
	return a
}
