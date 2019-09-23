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
		log(logging.Severity, interface{})
	} = stdlibLogger{}
)

// stackdriverLogger logs to GCP Stackdriver.
type stackdriverLogger struct {
	sdlogger *logging.Logger
}

func (l *stackdriverLogger) log(s logging.Severity, payload interface{}) {
	l.sdlogger.Log(logging.Entry{
		Severity: s,
		Payload:  payload,
	})
}

// stdlibLogger uses the Go standard library logger.
type stdlibLogger struct{}

func (stdlibLogger) log(s logging.Severity, payload interface{}) {
	log.Printf("%s: %+v", s, payload)
}

// UseStackdriver switches from the default stdlib logger to
// a Stackdriver logger. It assumes config.Init has been called.
// UseStackdriver returns the *logging.Logger it creates.
// UseStackdriver can only be called once. If it is called a second time, it returns an error.
func UseStackdriver(ctx context.Context, logName string) (_ *logging.Logger, err error) {
	defer derrors.Wrap(&err, "UseStackdriver(ctx, %q)", logName)

	client, err := logging.NewClient(ctx, config.ProjectID())
	if err != nil {
		return nil, err
	}
	l := client.Logger(logName, logging.CommonResource(config.AppMonitoredResource()))
	mu.Lock()
	defer mu.Unlock()
	if _, ok := logger.(*stackdriverLogger); ok {
		return nil, errors.New("already called once")
	}
	logger = &stackdriverLogger{l}
	return l, nil
}

// Infof logs a formatted string at the Info level.
func Infof(format string, args ...interface{}) { logf(logging.Info, format, args) }

// Errorf logs a formatted string at the Error level.
func Errorf(format string, args ...interface{}) { logf(logging.Error, format, args) }

// Debugf logs a formatted string at the Debug level.
func Debugf(format string, args ...interface{}) { logf(logging.Debug, format, args) }

// Fatalf is equivalent to Errorf followed by exiting the program.
func Fatalf(format string, args ...interface{}) {
	Errorf(format, args...)
	die()
}

func logf(s logging.Severity, format string, args []interface{}) {
	doLog(s, fmt.Sprintf(format, args...))
}

// Info logs arg, which can be a string or a struct, at the Info level.
func Info(arg interface{}) { doLog(logging.Info, arg) }

// Error logs arg, which can be a string or a struct, at the Error level.
func Error(arg interface{}) { doLog(logging.Error, arg) }

// Debug logs arg, which can be a string or a struct, at the Debug level.
func Debug(arg interface{}) { doLog(logging.Debug, arg) }

// Fatal is equivalent to Error followed by exiting the program.
func Fatal(arg interface{}) {
	Error(arg)
	die()
}

func doLog(s logging.Severity, payload interface{}) {
	mu.Lock()
	l := logger
	mu.Unlock()
	l.log(s, payload)
}

func die() {
	mu.Lock()
	if sl, ok := logger.(*stackdriverLogger); ok {
		sl.sdlogger.Flush()
	}
	mu.Unlock()
	os.Exit(1)
}
