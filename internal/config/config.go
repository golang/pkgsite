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
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ghodss/yaml"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal/derrors"
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

// AppVersionFormat is the expected format of the app version timestamp.
const AppVersionFormat = "20060102t150405"

// ValidateAppVersion validates that appVersion follows the expected format
// defined by AppVersionFormat.
func ValidateAppVersion(appVersion string) error {
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

// Config holds shared configuration values used in instantiating our server
// components.
type Config struct {
	// Discovery environment variables
	ProxyURL, IndexURL string

	// Ports used for hosting. 'DebugPort' is used for serving HTTP debug pages.
	Port, DebugPort string

	// AppEngine identifiers
	ProjectID, ServiceID, VersionID, ZoneID, InstanceID, LocationID string

	// QueueService is used to identify which service Cloud Tasks queue
	// should send requests to.
	QueueService string

	GaeEnv string

	// GoogleTagManagerID is the ID used for GoogleTagManager. It has the
	// structure GTM-XXXX.
	GoogleTagManagerID string

	// AppMonitoredResource is the resource for the current GAE app.
	// See https://cloud.google.com/monitoring/api/resources#tag_gae_app for more
	// details:
	// "An object representing a resource that can be used for monitoring, logging,
	// billing, or other purposes. Examples include virtual machine instances,
	// databases, and storage devices such as disks.""
	AppMonitoredResource *mrpb.MonitoredResource

	// FallbackVersionLabel is used as the VersionLabel when not hosting on
	// AppEngine.
	FallbackVersionLabel string

	DBSecret, DBUser, DBHost, DBPort, DBName string
	DBSecondaryHost                          string // DB host to use if first one is down
	DBPassword                               string `json:"-"`

	// Configuration for redis page cache.
	RedisCacheHost, RedisCachePort string

	// Configuration for redis autocompletion. This is different from the page
	// cache instance as it has different availability requirements.
	RedisHAHost, RedisHAPort string

	// UseProfiler specifies whether to enable Stackdriver Profiler.
	UseProfiler bool

	Quota QuotaSettings

	// TeeproxyTargetHosts is a list of hosts that teeproxy will forward
	// requests to.
	TeeproxyForwardedHosts []string
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
	return c.GaeEnv == "standard"
}

// StatementTimeout is the value of the Postgres statement_timeout parameter.
// Statements that run longer than this are terminated.
// 10 minutes is the App Engine standard request timeout.
const StatementTimeout = 10 * time.Minute

// SourceTimeout is the value of the timeout for source.Client, which is used
// to fetch source code from third party URLs.
const SourceTimeout = 1 * time.Minute

// TaskIDChangeIntervalWorker is the time period during which a given module
// version can be re-enqueued to fetch tasks.
const TaskIDChangeIntervalWorker = 3 * time.Hour

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

// configOverride holds selected config settings that can be dynamically overridden.
type configOverride struct {
	DBHost          string
	DBSecondaryHost string
	DBName          string
	Quota           QuotaSettings
}

// QuotaSettings is config for internal/middleware/quota.go
type QuotaSettings struct {
	QPS        int // allowed queries per second, per IP block
	Burst      int // maximum requests per second, per block; the size of the token bucket
	MaxEntries int // maximum number of entries to keep track of
	// Record data about blocking, but do not actually block.
	// This is a *bool, so we can distinguish "not present" from "false" in an override
	RecordOnly *bool
	// AcceptedURLs is the list of URLs that will be ignored by the quota
	// middleware.
	AcceptedURLs []string
}

const overrideBucket = "go-discovery"

// Init resolves all configuration values provided by the config package. It
// must be called before any configuration values are used.
func Init(ctx context.Context) (_ *Config, err error) {
	defer derrors.Add(&err, "config.Init(ctx)")
	// Build a Config from the execution environment, loading some values
	// from envvars and others from remote services.
	cfg := &Config{
		IndexURL:  GetEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index"),
		ProxyURL:  GetEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org"),
		Port:      os.Getenv("PORT"),
		DebugPort: os.Getenv("DEBUG_PORT"),
		// Resolve AppEngine identifiers
		ProjectID:          os.Getenv("GOOGLE_CLOUD_PROJECT"),
		ServiceID:          os.Getenv("GAE_SERVICE"),
		VersionID:          os.Getenv("GAE_VERSION"),
		InstanceID:         os.Getenv("GAE_INSTANCE"),
		GaeEnv:             os.Getenv("GAE_ENV"),
		GoogleTagManagerID: os.Getenv("GO_DISCOVERY_GOOGLE_TAG_MANAGER_ID"),
		QueueService:       GetEnv("GO_DISCOVERY_QUEUE_SERVICE", os.Getenv("GAE_SERVICE")),
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
		RedisHAHost:          os.Getenv("GO_DISCOVERY_REDIS_HA_HOST"),
		RedisHAPort:          GetEnv("GO_DISCOVERY_REDIS_HA_PORT", "6379"),
		Quota: QuotaSettings{
			QPS:          10,
			Burst:        20,
			MaxEntries:   1000,
			RecordOnly:   func() *bool { t := true; return &t }(),
			AcceptedURLs: parseCommaList(GetEnv("GO_DISCOVERY_ACCEPTED_LIST", "")),
		},
		UseProfiler:            os.Getenv("GO_DISCOVERY_USE_PROFILER") == "TRUE",
		TeeproxyForwardedHosts: parseCommaList(os.Getenv("GO_DISCOVERY_TEEPROXY_FORWARDED_HOSTS")),
	}
	cfg.AppMonitoredResource = &mrpb.MonitoredResource{
		Type: "gae_app",
		Labels: map[string]string{
			"project_id": cfg.ProjectID,
			"module_id":  cfg.ServiceID,
			"version_id": cfg.VersionID,
			"zone":       cfg.ZoneID,
		},
	}

	if cfg.GaeEnv != "" {
		// Zone is not available in the environment but can be queried via the metadata API.
		zone, err := gceMetadata(ctx, "instance/zone")
		if err != nil {
			return nil, err
		}
		cfg.ZoneID = zone
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

	// If GO_DISCOVERY_CONFIG_OVERRIDE is set, it should point to a file
	// in overrideBucket which provides overrides for selected configuration.
	// Use this when you want to fix something in prod quickly, without waiting
	// to re-deploy. (Otherwise, do not use it.)
	overrideObj := os.Getenv("GO_DISCOVERY_CONFIG_OVERRIDE")
	if overrideObj != "" {
		overrideBytes, err := readOverrideFile(ctx, overrideBucket, overrideObj)
		if err != nil {
			log.Print(err)
		} else {
			log.Printf("processing overrides from gs://%s/%s", overrideBucket, overrideObj)
			processOverrides(cfg, overrideBytes)
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

func processOverrides(cfg *Config, bytes []byte) {
	var ov configOverride
	if err := yaml.Unmarshal(bytes, &ov); err != nil {
		log.Printf("processOverrides: %v", err)
		return
	}
	overrideString("DBHost", &cfg.DBHost, ov.DBHost)
	overrideString("DBSecondaryHost", &cfg.DBSecondaryHost, ov.DBSecondaryHost)
	overrideString("DBName", &cfg.DBName, ov.DBName)
	overrideInt("Quota.QPS", &cfg.Quota.QPS, ov.Quota.QPS)
	overrideInt("Quota.Burst", &cfg.Quota.Burst, ov.Quota.Burst)
	overrideInt("Quota.MaxEntries", &cfg.Quota.MaxEntries, ov.Quota.MaxEntries)
	overrideBool("Quota.RecordOnly", &cfg.Quota.RecordOnly, ov.Quota.RecordOnly)
}

func overrideString(name string, field *string, val string) {
	if val != "" {
		*field = val
		log.Printf("overriding %s with %q", name, val)
	}
}

func overrideInt(name string, field *int, val int) {
	if val != 0 {
		*field = val
		log.Printf("overriding %s with %d", name, val)
	}
}

func overrideBool(name string, field **bool, val *bool) {
	if val != nil {
		*field = val
		log.Printf("overriding %s with %t", name, *val)
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
func gceMetadata(ctx context.Context, name string) (_ string, err error) {
	// See https://cloud.google.com/appengine/docs/standard/java/accessing-instance-metadata.
	// (This documentation doesn't exist for Golang, but it seems to work).
	defer derrors.Wrap(&err, "gceMetadata(ctx, %q)", name)

	const zoneURL = "http://metadata.google.internal/computeMetadata/v1/"
	req, err := http.NewRequest("GET", zoneURL+name, nil)
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
