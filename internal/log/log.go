// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package log supports structured and unstructured logging with levels.
package log

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"golang.org/x/pkgsite/internal/experiment"
)

type Severity int

const (
	SeverityDefault = Severity(iota)
	SeverityDebug
	SeverityInfo
	SeverityWarning
	SeverityError
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityDefault:
		return "Default"
	case SeverityDebug:
		return "Debug"
	case SeverityInfo:
		return "Info"
	case SeverityWarning:
		return "Warning"
	case SeverityError:
		return "Error"
	case SeverityCritical:
		return "Critical"
	default:
		return fmt.Sprint(int(s))
	}
}

type Logger interface {
	Log(ctx context.Context, s Severity, payload any)
	Flush()
}

var (
	mu     sync.Mutex
	logger Logger = stdlibLogger{}

	// currentLevel holds current log level.
	// No logs will be printed below currentLevel.
	currentLevel = SeverityDefault
)

// Set the log level
func SetLevel(v string) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = toLevel(v)
}

func getLevel() Severity {
	mu.Lock()
	defer mu.Unlock()
	return currentLevel
}

// stdlibLogger uses the Go standard library logger.
type stdlibLogger struct{}

func (stdlibLogger) Log(ctx context.Context, s Severity, payload any) {
	var extras []string
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

func (stdlibLogger) Flush() {}

func experimentString(ctx context.Context) string {
	return strings.Join(experiment.FromContext(ctx).Active(), ", ")
}

func Use(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	logger = l
}

// Infof logs a formatted string at the Info level.
func Infof(ctx context.Context, format string, args ...any) {
	logf(ctx, SeverityInfo, format, args)
}

// Warningf logs a formatted string at the Warning level.
func Warningf(ctx context.Context, format string, args ...any) {
	logf(ctx, SeverityWarning, format, args)
}

// Errorf logs a formatted string at the Error level.
func Errorf(ctx context.Context, format string, args ...any) {
	logf(ctx, SeverityError, format, args)
}

// Debugf logs a formatted string at the Debug level.
func Debugf(ctx context.Context, format string, args ...any) {
	logf(ctx, SeverityDebug, format, args)
}

// Fatalf logs formatted string at the Critical level followed by exiting the program.
func Fatalf(ctx context.Context, format string, args ...any) {
	logf(ctx, SeverityCritical, format, args)
	die()
}

func logf(ctx context.Context, s Severity, format string, args []any) {
	doLog(ctx, s, fmt.Sprintf(format, args...))
}

// Info logs arg, which can be a string or a struct, at the Info level.
func Info(ctx context.Context, arg any) { doLog(ctx, SeverityInfo, arg) }

// Warning logs arg, which can be a string or a struct, at the Warning level.
func Warning(ctx context.Context, arg any) { doLog(ctx, SeverityWarning, arg) }

// Error logs arg, which can be a string or a struct, at the Error level.
func Error(ctx context.Context, arg any) { doLog(ctx, SeverityError, arg) }

// Debug logs arg, which can be a string or a struct, at the Debug level.
func Debug(ctx context.Context, arg any) { doLog(ctx, SeverityDebug, arg) }

// Fatal logs arg, which can be a string or a struct, at the Critical level followed by exiting the program.
func Fatal(ctx context.Context, arg any) {
	doLog(ctx, SeverityCritical, arg)
	die()
}

func doLog(ctx context.Context, s Severity, payload any) {
	if getLevel() > s {
		return
	}
	mu.Lock()
	l := logger
	mu.Unlock()
	l.Log(ctx, s, payload)
}

func die() {
	mu.Lock()
	logger.Flush()
	mu.Unlock()
	os.Exit(1)
}

// toLevel returns the logging.Severity for a given string.
// Possible input values are "", "debug", "info", "warning", "error", "fatal".
// In case of invalid string input, it maps to DefaultLevel.
func toLevel(v string) Severity {
	v = strings.ToLower(v)

	switch v {
	case "":
		// default log level will print everything.
		return SeverityDefault
	case "debug":
		return SeverityDebug
	case "info":
		return SeverityInfo
	case "warning":
		return SeverityWarning
	case "error":
		return SeverityError
	case "fatal":
		return SeverityCritical
	}

	// Default log level in case of invalid input.
	log.Printf("Error: %s is invalid LogLevel. Possible values are [debug, info, warning, error, fatal]", v)
	return SeverityDefault
}
