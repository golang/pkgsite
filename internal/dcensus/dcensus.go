// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dcensus provides functionality for debug instrumentation.
package dcensus

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"go.opencensus.io/zpages"
	"golang.org/x/discovery/internal/config"
)

// Router is an http multiplexer that instruments per-handler debugging
// information and census instrumentation.
type Router struct {
	http.Handler
	mux *http.ServeMux
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	mux := http.NewServeMux()
	return &Router{
		mux:     mux,
		Handler: &ochttp.Handler{Handler: mux},
	}
}

// Handle registers handler with the given route. It has the same routing
// semantics as http.ServeMux.
func (r *Router) Handle(route string, handler http.Handler) {
	r.mux.Handle(route, ochttp.WithRouteTag(handler, route))
}

// HandleFunc is a wrapper around Handle for http.HandlerFuncs.
func (r *Router) HandleFunc(route string, handler http.HandlerFunc) {
	r.Handle(route, handler)
}

const debugPage = `
<html>
<p><a href="/tracez">/tracez</a> - trace spans</p>
<p><a href="/statsz">/statz</a> - prometheus metrics page</p>
`

// Init configures tracing and aggregation according to the given Views. If
// running on GCP, Init also configures exporting to StackDriver.
func Init(views ...*view.View) error {
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	if err := view.Register(views...); err != nil {
		return fmt.Errorf("view.Register: %v", err)
	}
	exportToStackdriver()
	return nil
}

// NewServer creates a new http.Handler for serving debug information.
func NewServer(views ...*view.View) (http.Handler, error) {
	pe, err := prometheus.NewExporter(prometheus.Options{})
	if err != nil {
		return nil, fmt.Errorf("prometheus.NewExporter: %v", err)
	}
	mux := http.NewServeMux()
	zpages.Handle(mux, "/")
	mux.Handle("/statsz", pe)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, debugPage)
	})

	return mux, nil
}

// ExportToStackdriver checks to see if the process is running in a GCP
// environment, and if so configures exporting to stackdriver.
func exportToStackdriver() {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		log.Printf("Not exporting to StackDriver: GOOGLE_CLOUD_PROJECT is unset.")
		return
	}

	// Report statistics every minutes, due to stackdriver limitations described at
	// https://cloud.google.com/monitoring/custom-metrics/creating-metrics#writing-ts
	view.SetReportingPeriod(time.Minute)

	labels := &stackdriver.Labels{}
	labels.Set("version_label", config.AppVersionLabel(), "Version label of the running binary")
	labels.Set("instance", config.InstanceID(), "Identifier of the executing instance")
	labels.Set("service", config.ServiceID(), "Identifier of the executing application")

	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID:               project,
		MonitoredResource:       monitoredresource.Autodetect(),
		DefaultMonitoringLabels: labels,
	})
	if err != nil {
		log.Fatal(err)
	}
	view.RegisterExporter(exporter)
	trace.RegisterExporter(exporter)
}

// ViewByCodeRouteMethod is a view of HTTP server requests parameterized by
// StatusCode, Route, and HTTP method.
var ViewByCodeRouteMethod = &view.View{
	Name:        "opencensus.io/http/server/response_count_by_status_code_route_method",
	Description: "Server response count by status code",
	TagKeys:     []tag.Key{ochttp.StatusCode, ochttp.KeyServerRoute, ochttp.Method},
	Measure:     ochttp.ServerLatency,
	Aggregation: view.Count(),
}
