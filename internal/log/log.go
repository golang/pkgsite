// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log supports structured and unstructured logging with levels.
package log

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"cloud.google.com/go/logging"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
)

var (
	mu     sync.Mutex
	logger interface {
		log(context.Context, logging.Severity, interface{})
	} = stdlibLogger{}
)

// traceIDKey is the type of the context key for trace IDs.
type traceIDKey struct{}

// NewContextWithTraceID creates a new context from ctx that adds the trace ID.
func NewContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// stackdriverLogger logs to GCP Stackdriver.
type stackdriverLogger struct {
	sdlogger *logging.Logger
}

func (l *stackdriverLogger) log(ctx context.Context, s logging.Severity, payload interface{}) {
	// Convert errors to strings, or they may serialize as the empty JSON object.
	if err, ok := payload.(error); ok {
		payload = err.Error()
	}
	traceID, _ := ctx.Value(traceIDKey{}).(string) // if not present, traceID is "", which is fine
	l.sdlogger.Log(logging.Entry{
		Severity: s,
		Payload:  payload,
		Trace:    traceID,
	})
}

// stdlibLogger uses the Go standard library logger.
type stdlibLogger struct{}

func (stdlibLogger) log(ctx context.Context, s logging.Severity, payload interface{}) {
	traceID, _ := ctx.Value(traceIDKey{}).(string) // if not present, traceID is ""
	if traceID != "" {
		log.Printf("%s (traceID %s): %+v", s, traceID, payload)
	} else {
		log.Printf("%s: %+v", s, payload)
	}
}

// UseStackdriver switches from the default stdlib logger to a Stackdriver
// logger. It assumes config.Init has been called. UseStackdriver returns a
// "parent" *logging.Logger that should be used to log the start and end of a
// request. It also creates and remembers internally a "child" logger that will
// be used to log within a request. The two loggers are necessary to get request-scoped
// logs in Stackdriver.
// See https://cloud.google.com/appengine/docs/standard/go/writing-application-logs.
//
// UseStackdriver can only be called once. If it is called a second time, it returns an error.
func UseStackdriver(ctx context.Context, cfg *config.Config, logName string) (_ *logging.Logger, err error) {
	defer derrors.Wrap(&err, "UseStackdriver(ctx, %q)", logName)

	client, err := logging.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	// Configure the cloud logger using the gae_app monitored resource. It would
	// be better to use the gae_instance monitored resource, but that's not
	// currently supported:
	// https://cloud.google.com/logging/docs/api/v2/resource-list#resource-types
	parent := client.Logger(logName, logging.CommonResource(cfg.AppMonitoredResource))
	child := client.Logger(logName+"-child", logging.CommonResource(cfg.AppMonitoredResource))
	mu.Lock()
	defer mu.Unlock()
	if _, ok := logger.(*stackdriverLogger); ok {
		return nil, errors.New("already called once")
	}
	logger = &stackdriverLogger{child}
	return parent, nil
}

// Infof logs a formatted string at the Info level.
func Infof(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, logging.Info, format, args)
}

// Errorf logs a formatted string at the Error level.
func Errorf(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, logging.Error, format, args)
}

// Debugf logs a formatted string at the Debug level.
func Debugf(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, logging.Debug, format, args)
}

// Fatalf is equivalent to Errorf followed by exiting the program.
func Fatalf(ctx context.Context, format string, args ...interface{}) {
	Errorf(ctx, format, args...)
	die()
}

func logf(ctx context.Context, s logging.Severity, format string, args []interface{}) {
	doLog(ctx, s, fmt.Sprintf(format, args...))
}

// Info logs arg, which can be a string or a struct, at the Info level.
func Info(ctx context.Context, arg interface{}) { doLog(ctx, logging.Info, arg) }

// Error logs arg, which can be a string or a struct, at the Error level.
func Error(ctx context.Context, arg interface{}) { doLog(ctx, logging.Error, arg) }

// Debug logs arg, which can be a string or a struct, at the Debug level.
func Debug(ctx context.Context, arg interface{}) { doLog(ctx, logging.Debug, arg) }

// Fatal is equivalent to Error followed by exiting the program.
func Fatal(ctx context.Context, arg interface{}) {
	Error(ctx, arg)
	die()
}

func doLog(ctx context.Context, s logging.Severity, payload interface{}) {
	mu.Lock()
	l := logger
	mu.Unlock()
	l.log(ctx, s, payload)
}

func die() {
	mu.Lock()
	if sl, ok := logger.(*stackdriverLogger); ok {
		sl.sdlogger.Flush()
	}
	mu.Unlock()
	os.Exit(1)
}
