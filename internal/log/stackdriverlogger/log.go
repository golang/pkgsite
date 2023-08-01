// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log supports structured and unstructured logging with levels
// to GCP stackdriver.
package stackdriverlogger

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/logging"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
)

func init() {
	// Log to stdout on GKE so the log messages are severity Info, rather than Error.
	if os.Getenv("GO_DISCOVERY_ON_GKE") != "" {
		// Question for the reviewer. Was this meant to be done for cmd/pkgsite? This
		// package won't be depended on by cmd/pkgsite, so that behavior will change, but
		// we don't want the core to have knowledge of the GO_DISCOVERY_ON_GKE variable.
		stdlog.SetOutput(os.Stdout)
	}
}

type (
	// traceIDKey is the type of the context key for trace IDs.
	traceIDKey struct{}

	// labelsKey is the type of the context key for labels.
	labelsKey struct{}
)

// NewContextWithTraceID creates a new context from ctx that adds the trace ID.
func NewContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// NewContextWithLabel creates a new context from ctx that adds a label that will
// appear in the log entry.
func NewContextWithLabel(ctx context.Context, key, value string) context.Context {
	oldLabels, _ := ctx.Value(labelsKey{}).(map[string]string)
	// Copy the labels, to preserve immutability of contexts.
	newLabels := map[string]string{}
	for k, v := range oldLabels {
		newLabels[k] = v
	}
	newLabels[key] = value
	return context.WithValue(ctx, labelsKey{}, newLabels)
}

// logger logs to GCP Stackdriver.
type logger struct {
	sdlogger *logging.Logger
}

func experimentString(ctx context.Context) string {
	return strings.Join(experiment.FromContext(ctx).Active(), ", ")
}

func stackdriverSeverity(s log.Severity) logging.Severity {
	switch s {
	case log.SeverityDefault:
		return logging.Default
	case log.SeverityDebug:
		return logging.Debug
	case log.SeverityInfo:
		return logging.Info
	case log.SeverityWarning:
		return logging.Warning
	case log.SeverityError:
		return logging.Error
	case log.SeverityCritical:
		return logging.Critical
	default:
		panic(fmt.Errorf("unknown severity: %v", s))
	}
}

func (l *logger) Log(ctx context.Context, s log.Severity, payload any) {
	// Convert errors to strings, or they may serialize as the empty JSON object.
	if err, ok := payload.(error); ok {
		payload = err.Error()
	}
	traceID, _ := ctx.Value(traceIDKey{}).(string) // if not present, traceID is "", which is fine
	labels, _ := ctx.Value(labelsKey{}).(map[string]string)
	es := experimentString(ctx)
	if len(es) > 0 {
		nl := map[string]string{}
		for k, v := range labels {
			nl[k] = v
		}
		nl["experiments"] = es
		labels = nl
	}
	l.sdlogger.Log(logging.Entry{
		Severity: stackdriverSeverity(s),
		Labels:   labels,
		Payload:  payload,
		Trace:    traceID,
	})
}

func (l *logger) Flush() {
	l.sdlogger.Flush()
}

var (
	mu            sync.Mutex
	alreadyCalled bool
)

// New creates a new Logger that logs to Stackdriver.
// It assumes config.Init has been called. New returns a
// "parent" *logging.Logger that should be used to log the start and end of a
// request. It also creates and remembers internally a "child" log.Logger that will
// be used to log within a request. The child logger should be passed to log.Use to
// forward the log package's logging calls to the child logger.
// The two loggers are necessary to get request-scoped  logs in Stackdriver.
// See https://cloud.google.com/appengine/docs/standard/go/writing-application-logs.
//
// New can only be called once. If it is called a second time, it returns an error.
func New(ctx context.Context, logName, projectID string, opts []logging.LoggerOption) (_ log.Logger, _ *logging.Logger, err error) {
	defer derrors.Wrap(&err, "New(ctx, %q)", logName)
	client, err := logging.NewClient(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	parent := client.Logger(logName, opts...)
	child := client.Logger(logName+"-child", opts...)
	mu.Lock()
	defer mu.Unlock()
	if alreadyCalled {
		return nil, nil, errors.New("already called once")
	}
	alreadyCalled = true
	return &logger{child}, parent, nil
}
