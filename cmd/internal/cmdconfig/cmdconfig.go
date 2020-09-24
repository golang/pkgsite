// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cmdconfig contains functions for configuring commands.
package cmdconfig

import (
	"context"
	"os"
	"time"

	"cloud.google.com/go/errorreporting"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/config/dynconfig"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
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
	if !cfg.OnGCP() {
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
	return reporter
}

// Experimenter configures a middleware.Experimenter.
func Experimenter(ctx context.Context, cfg *config.Config, getter middleware.ExperimentGetter, reportingClient *errorreporting.Client) *middleware.Experimenter {
	if os.Getenv("GO_DISCOVERY_EXPERIMENTS_FROM_CONFIG") == "true" {
		// Ignore getter, use dynamic config.
		if cfg.DynamicConfigLocation == "" {
			log.Warningf(ctx, "experiments are not configured")
			getter = func(context.Context) ([]*internal.Experiment, error) { return nil, nil }
		} else {
			log.Infof(ctx, "using dynamic config from %s for experiments", cfg.DynamicConfigLocation)
			getter = func(ctx context.Context) ([]*internal.Experiment, error) {
				dc, err := dynconfig.Read(ctx, cfg.DynamicConfigLocation)
				if err != nil {
					return nil, err
				}
				return dc.Experiments, nil
			}
		}
	}
	e, err := middleware.NewExperimenter(ctx, 1*time.Minute, getter, reportingClient)
	if err != nil {
		log.Fatal(ctx, err)
	}
	return e
}
