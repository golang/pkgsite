// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The prober hits the frontend with a fixed set of URLs.
// It is designed to be run periodically and to export
// metrics for altering and performance tracking.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"text/template"
	"time"

	"cloud.google.com/go/logging"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/metric/metricexport"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/pkgsite/internal/auth"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/log"
)

var credsFile = flag.String("creds", "", "filename for credentials, when running locally")

// A Probe represents a single HTTP GET request.
type Probe struct {
	// A short, stable name for the probe.
	// Since it is used in metrics, it shouldn't be too long and
	// should stay the same even if actual URL changes.
	Name string

	// The part of the URL after the host:port.
	RelativeURL string

	// Whether or not to set a header that causes the frontend to skip the redis
	// cache.
	BypassCache bool
}

var probes = []*Probe{
	{
		Name:        "home",
		RelativeURL: "",
	},
	{
		Name:        "search-help",
		RelativeURL: "search-help",
	},
	{
		Name:        "license-policy",
		RelativeURL: "license-policy",
	},
	{
		Name:        "pkg-firestore",
		RelativeURL: "cloud.google.com/go/firestore",
	},
	{
		Name:        "pkg-firestore-nocache",
		RelativeURL: "cloud.google.com/go/firestore",
		BypassCache: true,
	},
	{
		Name:        "pkg-firestore-versions",
		RelativeURL: "cloud.google.com/go/firestore?tab=versions",
	},
	{
		Name:        "pkg-firestore-versions-nocache",
		RelativeURL: "cloud.google.com/go/firestore?tab=versions",
		BypassCache: true,
	},
	{
		Name:        "pkg-firestore-imports",
		RelativeURL: "cloud.google.com/go/firestore?tab=imports",
	},
	{
		Name:        "pkg-firestore-imports-nocache",
		RelativeURL: "cloud.google.com/go/firestore?tab=imports",
		BypassCache: true,
	},
	{
		Name:        "pkg-firestore-importedby",
		RelativeURL: "cloud.google.com/go/firestore?tab=importedby",
	},
	{
		Name:        "pkg-firestore-importedby-nocache",
		RelativeURL: "cloud.google.com/go/firestore?tab=importedby",
		BypassCache: true,
	},
	{
		Name:        "pkg-firestore-licenses",
		RelativeURL: "cloud.google.com/go/firestore?tab=licenses",
	},
	{
		Name:        "pkg-firestore-licenses-nocache",
		RelativeURL: "cloud.google.com/go/firestore?tab=licenses",
		BypassCache: true,
	},
	{
		Name:        "pkg-errors-importedby",
		RelativeURL: "github.com/pkg/errors?tab=importedby",
	},
	{
		Name:        "pkg-errors-importedby-nocache",
		RelativeURL: "github.com/pkg/errors?tab=importedby",
		BypassCache: true,
	},
	{
		Name:        "pkg-hortonworks-versions",
		RelativeURL: "github.com/hortonworks/cb-cli?tab=versions",
		BypassCache: true,
	},
	{
		Name:        "pkg-xtoolsgo-directory",
		RelativeURL: "golang.org/x/tools/go",
		BypassCache: true,
	},
	{
		Name:        "xtools-nocache",
		RelativeURL: "golang.org/x/tools",
		BypassCache: true,
	},
	{
		Name:        "xtools-versions-nocache",
		RelativeURL: "golang.org/x/tools?tab=versions",
		BypassCache: true,
	},
	{
		Name:        "xtools-licenses-nocache",
		RelativeURL: "golang.org/x/tools?tab=licenses",
		BypassCache: true,
	},
	{
		Name:        "search-github",
		RelativeURL: "search?q=github",
	},
	{
		Name:        "search-github-nocache",
		RelativeURL: "search?q=github",
		BypassCache: true,
	},
}

func init() {
	// Validate that probe names are unique.
	names := map[string]bool{}
	for _, p := range probes {
		if names[p.Name] {
			log.Fatalf(context.Background(), "duplicate probe name %q", p.Name)
		}
		names[p.Name] = true
	}
}

var (
	baseURL        string
	authValue      string
	client         *http.Client
	metricExporter *stackdriver.Exporter
	metricReader   *metricexport.Reader
	keyName        = tag.MustNewKey("probe.name")
	keyStatus      = tag.MustNewKey("probe.status")

	firstByteLatency = stats.Float64(
		"go-discovery/first_byte_latency",
		"Time between first byte of request headers sent to first byte of response received, or error",
		stats.UnitMilliseconds,
	)

	firstByteLatencyDistribution = &view.View{
		Name:        "go-discovery/prober/first_byte_latency",
		Measure:     firstByteLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "first-byte latency, by probe name and response status",
		TagKeys:     []tag.Key{keyName, keyStatus},
	}

	probeCount = &view.View{
		Name:        "go-discovery/prober/probe_count",
		Measure:     firstByteLatency,
		Aggregation: view.Count(),
		Description: "probe count, by probe name and response status",
		TagKeys:     []tag.Key{keyName, keyStatus},
	}
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	ctx := context.Background()

	baseURL = config.GetEnv("PROBER_BASE_URL", "")
	if baseURL == "" {
		log.Fatal(ctx, "must set PROBER_BASE_URL")
	}
	log.Infof(ctx, "base URL %s", baseURL)

	authValue = config.GetEnv("GO_DISCOVERY_PROBER_AUTH_VALUE", "")
	if authValue == "" {
		log.Warningf(ctx, "missing GO_DISCOVERY_PROBER_AUTH_VALUE; won't bypass cache or quota")
	}

	cfg, err := config.Init(ctx)
	if err != nil {
		log.Fatal(ctx, err)
	}
	cfg.Dump(os.Stderr)

	if cfg.OnGCP() {
		opts := []logging.LoggerOption{logging.CommonResource(cfg.MonitoredResource)}
		if _, err := log.UseStackdriver(ctx, "prober-log", cfg.ProjectID, opts); err != nil {
			log.Fatal(ctx, err)
		}
	}

	var jsonCreds []byte
	if *credsFile != "" {
		jsonCreds, err = ioutil.ReadFile(*credsFile)
		if err != nil {
			log.Fatal(ctx, err)
		}
	}
	// If there is no creds file, use application default credentials. On
	// AppEngine, this will use the AppEngine service account, which has the
	// necessary IAP permission.
	client, err = auth.NewClient(ctx, jsonCreds, os.Getenv("GO_DISCOVERY_USE_EXP_AUTH") == "true")
	if err != nil {
		log.Fatal(ctx, err)
	}

	if err := view.Register(firstByteLatencyDistribution, probeCount); err != nil {
		log.Fatalf(ctx, "view.Register: %v", err)
	}
	metricExporter, err = dcensus.NewViewExporter(cfg)
	if err != nil {
		log.Fatal(ctx, err)
	}

	// To export metrics immediately, we use a metric reader.  See runProbes, below.
	metricReader = metricexport.NewReader()

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/shared/icon/favicon.ico")
	})
	http.HandleFunc("/", handleProbe)
	http.HandleFunc("/check", handleCheck)

	addr := cfg.HostAddr("localhost:8080")
	log.Infof(ctx, "Listening on addr %s", addr)
	log.Fatal(ctx, http.ListenAndServe(addr, nil))
}

// ProbeStatus records the result if a single probe attempt
type ProbeStatus struct {
	Probe   *Probe
	Code    int    // status code of response
	Text    string // describes what happened: "OK", or "FAILED" with a reason
	Latency int    // in milliseconds
}

// handleProbe runs probes and displays their results. It always returns a 200.
func handleProbe(w http.ResponseWriter, r *http.Request) {
	statuses := runProbes(r.Context())
	var data = struct {
		Start    time.Time
		BaseURL  string
		Statuses []*ProbeStatus
	}{
		Start:    time.Now(),
		BaseURL:  baseURL,
		Statuses: statuses,
	}
	var buf bytes.Buffer
	err := statusTemplate.Execute(&buf, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("template execution failed: %v", err), http.StatusInternalServerError)
	} else {
		buf.WriteTo(w) // ignore error; nothing we can do about it
	}
}

// handleCheck runs probes, and returns a 200 only if they all succeed.
// Otherwise it returns the status code of the first failing response.
func handleCheck(w http.ResponseWriter, r *http.Request) {
	statuses := runProbes(r.Context())
	var bads []*ProbeStatus
	for _, s := range statuses {
		if s.Code != http.StatusOK {
			bads = append(bads, s)
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	if len(bads) == 0 {
		fmt.Fprintf(w, "All probes succeeded.\n")
	} else {
		w.WriteHeader(bads[0].Code)
		fmt.Fprintf(w, "SOME PROBES FAILED:\n")
		for _, b := range bads {
			fmt.Fprintf(w, "%3d /%s\n", b.Code, b.Probe.RelativeURL)
		}
	}
}

func runProbes(ctx context.Context) []*ProbeStatus {
	var statuses []*ProbeStatus
	for _, p := range probes {
		s := runProbe(ctx, p)
		statuses = append(statuses, s)
	}
	metricReader.ReadAndExport(metricExporter)
	metricExporter.Flush()
	log.Info(ctx, "metrics exported to StackDriver")
	return statuses
}

func runProbe(ctx context.Context, p *Probe) *ProbeStatus {
	status := &ProbeStatus{
		Probe: p,
		Code:  499, // not a real code; means request never sent
	}
	url := baseURL + "/" + p.RelativeURL
	log.Infof(ctx, "running %s = %s", p.Name, url)
	defer func() {
		log.Infof(ctx, "%s in %dms", status.Text, status.Latency)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if authValue != "" {
		if p.BypassCache {
			req.Header.Set(config.BypassCacheAuthHeader, authValue)
		}
		req.Header.Set(config.BypassQuotaAuthHeader, authValue)
	}
	if err != nil {
		status.Text = fmt.Sprintf("FAILED making request: %v", err)
		return status
	}
	start := time.Now()
	res, err := client.Do(req.WithContext(ctx))

	latency := float64(time.Since(start)) / float64(time.Millisecond)
	status.Latency = int(latency)
	record := func(statusTag string) {
		stats.RecordWithTags(ctx, []tag.Mutator{
			tag.Upsert(keyName, p.Name),
			tag.Upsert(keyStatus, statusTag),
		}, firstByteLatency.M(latency))
	}

	if err != nil {
		status.Text = fmt.Sprintf("FAILED call: %v", err)
		record("FAILED call")
		return status
	}
	defer res.Body.Close()
	status.Code = res.StatusCode
	if res.StatusCode != http.StatusOK {
		status.Text = fmt.Sprintf("FAILED with status %s", res.Status)
		record(res.Status)
		return status
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		status.Text = fmt.Sprintf("FAILED reading body: %v", err)
		record("FAILED read body")
		return status
	}
	if !bytes.Contains(body, []byte("go.dev")) {
		status.Text = "FAILED: body does not contain 'go.dev'"
		record("FAILED wrong body")
		return status
	}
	status.Text = "OK"
	record("200 OK")
	return status
}

var statusTemplate = template.Must(template.New("").Parse(`
<html>
  <head>
    <title>Go Discovery Prober</title>
  </head>
  <body>
    <h1>Probes at at {{with .Start}}{{.Format "2006-1-2 15:04"}}{{end}}</h1>
    Base URL: {{.BaseURL}}<br/>
    <table cellspacing="10rem">
      <tr><th>Name</th><th>URL</th><th>Latency (ms)</th><th>Status</th></tr>
      {{range .Statuses}}
        <tr>
          <td>{{.Probe.Name}}</td>
          <td>{{.Probe.RelativeURL}}</td>
          <td>{{.Latency}}</td>
          <td>{{.Text}}</td>
        </tr>
      {{end}}
    </table>
  </body>
</html>
`))
