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
	"strings"
	"sync"

	"cloud.google.com/go/logging"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
)

var (
	mu     sync.Mutex
	logger interface {
		log(context.Context, logging.Severity, interface{})
	} = stdlibLogger{}

	// currentLevel holds current log level.
	// No logs will be printed below currentLevel.
	currentLevel = logging.Default
)

type (
	// traceIDKey is the type of the context key for trace IDs.
	traceIDKey struct{}

	// labelsKey is the type of the context key for labels.
	labelsKey struct{}
)

// Set the log level
func SetLevel(v string) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = toLevel(v)
}

func getLevel() logging.Severity {
	mu.Lock()
	defer mu.Unlock()
	return currentLevel
}

// NewContextWithTraceID creates a new context from ctx that adds the trace ID.
func NewContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// NewContextWithLabel creates anew context from ctx that adds a label that will
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
		Severity: s,
		Labels:   labels,
		Payload:  payload,
		Trace:    traceID,
	})
}

// stdlibLogger uses the Go standard library logger.
type stdlibLogger struct{}

func (stdlibLogger) log(ctx context.Context, s logging.Severity, payload interface{}) {
	var extras []string
	traceID, _ := ctx.Value(traceIDKey{}).(string) // if not present, traceID is ""
	if traceID != "" {
		extras = append(extras, fmt.Sprintf("traceID %s", traceID))
	}
	if labels, ok := ctx.Value(labelsKey{}).(map[string]string); ok {
		extras = append(extras, fmt.Sprint(labels))
	}
	es := experimentString(ctx)
	if len(es) > 0 {
		extras = append(extras, fmt.Sprintf("experiments %s", es))
	}
	var extra string
	if len(extras) > 0 {
		extra = " (" + strings.Join(extras, ", ") + ")"
	}
	log.Printf("%s%s: %+v", s, extra, payload)

}

func experimentString(ctx context.Context) string {
	return strings.Join(experiment.FromContext(ctx).Active(), ", ")
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
	parent := client.Logger(logName, logging.CommonResource(cfg.MonitoredResource))
	child := client.Logger(logName+"-child", logging.CommonResource(cfg.MonitoredResource))
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

// Fatalf logs formatted string at the Critical level followed by exiting the program.
func Fatalf(ctx context.Context, format string, args ...interface{}) {
	logf(ctx, logging.Critical, format, args)
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

// Fatal logs arg, which can be a string or a struct, at the Critical level followed by exiting the program.
func Fatal(ctx context.Context, arg interface{}) {
	doLog(ctx, logging.Critical, arg)
	die()
}

func doLog(ctx context.Context, s logging.Severity, payload interface{}) {
	if getLevel() > s {
		return
	}
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

// toLevel returns the logging.Severity for a given string.
// Possible input values are "", "debug", "info", "error", "fatal".
// In case of invalid string input, it maps to DefaultLevel.
func toLevel(v string) logging.Severity {
	v = strings.ToLower(v)

	switch v {
	case "":
		// default log level will print everything.
		return logging.Default
	case "debug":
		return logging.Debug
	case "info":
		return logging.Info
	case "error":
		return logging.Error
	case "fatal":
		return logging.Critical
	}

	// Default log level in case of invalid input.
	log.Printf("Error: %s is invalid LogLevel. Possible values are [debug, info, error, fatal]", v)
	return logging.Default
}
