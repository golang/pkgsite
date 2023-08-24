// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package serverconfig resolves shared configuration for Go Discovery services.
package serverconfig

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/secrets"
	"gopkg.in/yaml.v3"
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

// ValidateAppVersion validates that appVersion follows the expected format
// defined by AppVersionFormat.
func ValidateAppVersion(appVersion string) error {
	// Accept GKE versions, which start with the docker image name.
	if strings.HasPrefix(appVersion, "gcr.io/") {
		return nil
	}
	if _, err := time.Parse(config.AppVersionFormat, appVersion); err != nil {
		// Accept alternative version, used by our AppEngine deployment script.
		const altDateFormat = "2006-01-02t15-04"
		if len(appVersion) > len(altDateFormat) {
			appVersion = appVersion[:len(altDateFormat)]
		}
		if _, err := time.Parse(altDateFormat, appVersion); err != nil {
			return fmt.Errorf("app version %q does not match time formats %q or %q: %v",
				appVersion, config.AppVersionFormat, altDateFormat, err)
		}
	}
	return nil
}

// OnAppEngine reports if the current process is running in an AppEngine
// environment.
func OnAppEngine() bool {
	return os.Getenv("GAE_ENV") == "standard"
}

// OnGKE reports whether the current process is running on GKE.
func OnGKE() bool {
	return os.Getenv("GO_DISCOVERY_ON_GKE") == "true"
}

// onCloudRun reports whether the current process is running on Cloud Run.
func onCloudRun() bool {
	// Use the presence of the environment variables provided by Cloud Run.
	// See https://cloud.google.com/run/docs/reference/container-contract.
	for _, ev := range []string{"K_SERVICE", "K_REVISION", "K_CONFIGURATION"} {
		if os.Getenv(ev) == "" {
			return false
		}
	}
	return true
}

// OnGCP reports whether the current process is running on Google Cloud
// Platform.
func OnGCP() bool {
	return OnAppEngine() || OnGKE() || onCloudRun()
}

// configOverride holds selected config settings that can be dynamically overridden.
type configOverride struct {
	DBHost          string               `yaml:"DBHost"`
	DBSecondaryHost string               `yaml:"DBSecondaryHost"`
	DBName          string               `yaml:"DBName"`
	Quota           config.QuotaSettings `yaml:"Quota"`
}

// Init resolves all configuration values provided by the config package. It
// must be called before any configuration values are used.
func Init(ctx context.Context) (_ *config.Config, err error) {
	defer derrors.Add(&err, "config.Init(ctx)")
	// Build a Config from the execution environment, loading some values
	// from envvars and others from remote services.
	cfg := &config.Config{
		AuthValues: parseCommaList(os.Getenv("GO_DISCOVERY_AUTH_VALUES")),
		IndexURL:   GetEnv("GO_MODULE_INDEX_URL", "https://index.golang.org/index"),
		ProxyURL:   GetEnv("GO_MODULE_PROXY_URL", "https://proxy.golang.org"),
		Port:       os.Getenv("PORT"),
		DebugPort:  os.Getenv("DEBUG_PORT"),
		// Resolve AppEngine identifiers
		ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT"),
		ServiceID: GetEnv("GAE_SERVICE", os.Getenv("GO_DISCOVERY_SERVICE")),
		// Version ID from either AppEngine, Cloud Run (see
		// https://cloud.google.com/run/docs/reference/container-contract) or
		// GKE (set by our own config).
		VersionID:          GetEnv("GAE_VERSION", GetEnv("K_REVISION", os.Getenv("DOCKER_IMAGE"))),
		InstanceID:         GetEnv("GAE_INSTANCE", os.Getenv("GO_DISCOVERY_INSTANCE")),
		GoogleTagManagerID: os.Getenv("GO_DISCOVERY_GOOGLE_TAG_MANAGER_ID"),
		QueueURL:           os.Getenv("GO_DISCOVERY_QUEUE_URL"),
		QueueAudience:      os.Getenv("GO_DISCOVERY_QUEUE_AUDIENCE"),

		// LocationID is essentially hard-coded until we figure out a good way to
		// determine it programmatically, but we check an environment variable in
		// case it needs to be overridden.
		LocationID: GetEnv("GO_DISCOVERY_GAE_LOCATION_ID", "us-central1"),
		// This fallback should only be used when developing locally.
		FallbackVersionLabel: time.Now().Format(config.AppVersionFormat),
		DBHost:               chooseOne(GetEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")),
		DBUser:               GetEnv("GO_DISCOVERY_DATABASE_USER", "postgres"),
		DBPassword:           os.Getenv("GO_DISCOVERY_DATABASE_PASSWORD"),
		DBSecondaryHost:      chooseOne(os.Getenv("GO_DISCOVERY_DATABASE_SECONDARY_HOST")),
		DBPort:               GetEnv("GO_DISCOVERY_DATABASE_PORT", "5432"),
		DBName:               GetEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-db"),
		DBSecret:             os.Getenv("GO_DISCOVERY_DATABASE_SECRET"),
		DBSSL:                GetEnv("GO_DISCOVERY_DATABASE_SSL", "disable"),
		RedisCacheHost:       os.Getenv("GO_DISCOVERY_REDIS_HOST"),
		RedisBetaCacheHost:   os.Getenv("GO_DISCOVERY_REDIS_BETA_HOST"),
		RedisCachePort:       GetEnv("GO_DISCOVERY_REDIS_PORT", "6379"),
		Quota: config.QuotaSettings{
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
	configDynamic := os.Getenv("GO_DISCOVERY_CONFIG_DYNAMIC")
	exclude := os.Getenv("GO_DISCOVERY_EXCLUDED_FILENAME")
	if bucket != "" {
		if configDynamic == "" {
			return nil, errors.New("GO_DISCOVERY_CONFIG_DYNAMIC must be set if GO_DISCOVERY_CONFIG_BUCKET is")
		}
		cfg.DynamicConfigLocation = fmt.Sprintf("gs://%s/%s", bucket, configDynamic)
		if exclude != "" {
			cfg.DynamicExcludeLocation = fmt.Sprintf("gs://%s/%s", bucket, exclude)
		}
	} else {
		cfg.DynamicConfigLocation = configDynamic
		cfg.DynamicExcludeLocation = exclude
	}
	if OnGCP() {
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
		case OnAppEngine():
			// Use the gae_app monitored resource. It would be better to use the
			// gae_instance monitored resource, but that's not currently supported:
			// https://cloud.google.com/logging/docs/api/v2/resource-list#resource-types
			cfg.MonitoredResource = &config.MonitoredResource{
				Type: "gae_app",
				Labels: map[string]string{
					"project_id": cfg.ProjectID,
					"module_id":  cfg.ServiceID,
					"version_id": cfg.VersionID,
					"zone":       cfg.ZoneID,
				},
			}
		case onCloudRun():
			cfg.MonitoredResource = &config.MonitoredResource{
				Type: "cloud_run_revision",
				Labels: map[string]string{
					"project_id":         cfg.ProjectID,
					"service_name":       cfg.ServiceID,
					"revision_name":      cfg.VersionID,
					"configuration_name": os.Getenv("K_CONFIGURATION"),
				},
			}
		case OnGKE():
			cfg.MonitoredResource = &config.MonitoredResource{
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
		if cfg.InstanceID == "" {
			id, err := gceMetadata(ctx, "instance/id")
			if err != nil {
				return nil, fmt.Errorf("getting instance ID: %v", err)
			}
			cfg.InstanceID = id
		}
	} else { // running locally, perhaps
		cfg.MonitoredResource = &config.MonitoredResource{
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
		log.Debugf(ctx, "quota enforcement enabled: qps=%d burst=%d maxentry=%d", cfg.Quota.QPS, cfg.Quota.Burst, cfg.Quota.MaxEntries)
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
	return io.ReadAll(r)
}

func processOverrides(ctx context.Context, cfg *config.Config, bytes []byte) {
	var ov configOverride
	if err := yaml.Unmarshal(bytes, &ov); err != nil {
		log.Errorf(ctx, "processOverrides: yaml.Unmarshal: %v", err)
		return
	}
	override(ctx, "DBHost", &cfg.DBHost, ov.DBHost)
	override(ctx, "DBSecondaryHost", &cfg.DBSecondaryHost, ov.DBSecondaryHost)
	override(ctx, "DBName", &cfg.DBName, ov.DBName)
	override(ctx, "Quota.QPS", &cfg.Quota.QPS, ov.Quota.QPS)
	override(ctx, "Quota.Burst", &cfg.Quota.Burst, ov.Quota.Burst)
	override(ctx, "Quota.MaxEntries", &cfg.Quota.MaxEntries, ov.Quota.MaxEntries)
	override(ctx, "Quota.RecordOnly", &cfg.Quota.RecordOnly, ov.Quota.RecordOnly)
}

func override[T comparable](ctx context.Context, name string, field *T, val T) {
	var zero T
	if val != zero {
		*field = val
		log.Infof(ctx, "overriding %s with %v", name, val)
	}
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
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("io.ReadAll: %v", err)
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
