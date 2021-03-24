// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cmdconfig contains functions for configuring commands.
package cmdconfig

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/errorreporting"
	"contrib.go.opencensus.io/integrations/ocsql"
	_ "github.com/jackc/pgx/v4/stdlib" // for pgx driver
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/dynconfig"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
)

// Logger configures a middleware.Logger.
func Logger(ctx context.Context, cfg *config.Config, logName string) middleware.Logger {
	if cfg.OnGCP() {
		logger, err := log.UseStackdriver(ctx, cfg, logName)
		if err != nil {
			log.Fatal(ctx, err)
		}
		return logger
	}
	return middleware.LocalLogger{}
}

// ReportingClient configures an Error Reporting client.
func ReportingClient(ctx context.Context, cfg *config.Config) *errorreporting.Client {
	if !cfg.OnGCP() || cfg.DisableErrorReporting {
		return nil
	}
	reporter, err := errorreporting.NewClient(ctx, cfg.ProjectID, errorreporting.Config{
		ServiceName: cfg.ServiceID,
		OnError: func(err error) {
			log.Errorf(ctx, "Error reporting failed: %v", err)
		},
	})
	if err != nil {
		log.Fatal(ctx, err)
	}
	derrors.SetReportingClient(reporter)
	return reporter
}

// Experimenter configures a middleware.Experimenter.
func Experimenter(ctx context.Context, cfg *config.Config, getter middleware.ExperimentGetter, reportingClient *errorreporting.Client) *middleware.Experimenter {
	e, err := middleware.NewExperimenter(ctx, 1*time.Minute, getter, reportingClient)
	if err != nil {
		log.Fatal(ctx, err)
	}
	return e
}

// ExperimentGetter returns an ExperimentGetter using the config.
func ExperimentGetter(ctx context.Context, cfg *config.Config) middleware.ExperimentGetter {
	if cfg.DynamicConfigLocation == "" {
		log.Warningf(ctx, "experiments are not configured")
		return func(context.Context) ([]*internal.Experiment, error) { return nil, nil }
	}
	log.Infof(ctx, "using dynamic config from %s for experiments", cfg.DynamicConfigLocation)
	return func(ctx context.Context) ([]*internal.Experiment, error) {
		dc, err := dynconfig.Read(ctx, cfg.DynamicConfigLocation)
		if err != nil {
			return nil, err
		}

		var s []string
		for _, e := range dc.Experiments {
			s = append(s, fmt.Sprintf("%s:%d", e.Name, e.Rollout))
			if desc, ok := internal.Experiments[e.Name]; ok {
				if e.Description == "" {
					e.Description = desc
				}
			} else {
				log.Errorf(ctx, "unknown experiment %q", e.Name)
			}
		}
		log.Infof(ctx, "read experiments %s", strings.Join(s, ", "))
		return dc.Experiments, nil
	}
}

// OpenDB opens the postgres database specified by the config.
// It first tries the main connection info (DBConnInfo), and if that fails, it uses backup
// connection info it if exists (DBSecondaryConnInfo).
func OpenDB(ctx context.Context, cfg *config.Config, bypassLicenseCheck bool) (_ *postgres.DB, err error) {
	defer derrors.Wrap(&err, "cmdconfig.OpenDB(ctx, cfg)")

	// Wrap the postgres driver with our own wrapper, which adds OpenCensus instrumentation.
	ocDriver, err := database.RegisterOCWrapper(cfg.DBDriver, ocsql.WithAllTraceOptions())
	if err != nil {
		return nil, fmt.Errorf("unable to register the ocsql driver: %v", err)
	}
	log.Infof(ctx, "opening database on host %s", cfg.DBHost)
	ddb, err := database.Open(ocDriver, cfg.DBConnInfo(), cfg.InstanceID)
	if err == nil {
		log.Infof(ctx, "connected to primary host: %s", cfg.DBHost)
	} else {
		ci := cfg.DBSecondaryConnInfo()
		if ci == "" {
			log.Infof(ctx, "no secondary DB host")
			return nil, err
		}
		log.Errorf(ctx, "database.Open for primary host %s failed with %v; trying secondary host %s ",
			cfg.DBHost, err, cfg.DBSecondaryHost)
		ddb, err = database.Open(ocDriver, ci, cfg.InstanceID)
		if err != nil {
			return nil, err
		}
		log.Infof(ctx, "connected to secondary host %s", cfg.DBSecondaryHost)
	}
	log.Infof(ctx, "database open finished")
	if bypassLicenseCheck {
		return postgres.NewBypassingLicenseCheck(ddb), nil
	}
	return postgres.New(ddb), nil
}
