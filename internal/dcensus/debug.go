// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dcensus

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal/log"
)

type debugTraceExporter struct {
	exp trace.Exporter
	mu  sync.Mutex
	err error
}

func (d *debugTraceExporter) onError(err error) {
	log.Debugf(context.Background(), "trace exporter: onError called with %v", err)
	d.err = err
}

// ExportSpan implements the trace.Exporter interface.
func (d *debugTraceExporter) ExportSpan(s *trace.SpanData) {
	ctx := context.Background()
	d.mu.Lock()
	d.exp.ExportSpan(s)
	err := d.err
	d.err = nil
	d.mu.Unlock()
	if err != nil {
		log.Warningf(ctx, "trace exporter: %v", err)
		log.Debugf(ctx, "trace exporter SpanData:\n%s", dumpSpanData(s))
	}
}

func dumpSpanData(s *trace.SpanData) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Name: %q\n", s.Name)
	dumpAttributes(&buf, s.Attributes)
	for _, a := range s.Annotations {
		fmt.Fprintf(&buf, "  annotation: %q\n", a.Message)
		dumpAttributes(&buf, a.Attributes)
	}
	fmt.Fprintf(&buf, "Status.Message: %q\n", s.Status.Message)
	fmt.Fprintln(&buf, "link attrs:")
	for _, l := range s.Links {
		dumpAttributes(&buf, l.Attributes)
	}
	return buf.String()
}

func dumpAttributes(w io.Writer, m map[string]any) {
	for k, v := range m {
		fmt.Fprintf(w, "  %q: %#v\n", k, v)
	}
}
