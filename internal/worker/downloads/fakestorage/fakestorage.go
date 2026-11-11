// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fakestorage implements the subset of the GCS HTTP API needed to test gcsfs.
package fakestorage

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"time"

	raw "google.golang.org/api/storage/v1"
)

// NewServer starts and returns a new [httptest.Server] serving the given bucket
// backed by fsys. Pass srv.URL (or srv.Listener.Addr().String()) in the
// STORAGE_EMULATOR_HOST environment variable so cloud.google.com/go/storage
// connects to it without authentication.
func NewServer(bucket string, fsys fs.FS) *httptest.Server {
	return httptest.NewServer(NewHandler(bucket, fsys))
}

// NewHandler returns an [http.Handler] implementing the GCS JSON list endpoint
// and XML object download endpoint for bucket backed by fsys.
func NewHandler(bucket string, fsys fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /storage/v1/b/{bucket}/o", func(w http.ResponseWriter, r *http.Request) {
		listObjects(w, r, bucket, fsys)
	})
	mux.HandleFunc("GET /{bucket}/{object...}", func(w http.ResponseWriter, r *http.Request) {
		serveObject(w, r, bucket, fsys)
	})
	return mux
}

func listObjects(w http.ResponseWriter, r *http.Request, bucket string, fsys fs.FS) {
	if r.PathValue("bucket") != bucket {
		http.NotFound(w, r)
		return
	}
	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")

	prefixSet := make(map[string]bool)
	var items []*raw.Object

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(p, prefix) {
			return nil
		}
		rest := strings.TrimPrefix(p, prefix)
		if delimiter != "" {
			if before, _, ok := strings.Cut(rest, delimiter); ok {
				prefixSet[prefix+before+delimiter] = true
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		items = append(items, &raw.Object{
			Name:    p,
			Size:    uint64(info.Size()),
			Updated: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := &raw.Objects{
		Items:    items,
		Prefixes: slices.Sorted(maps.Keys(prefixSet)),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func serveObject(w http.ResponseWriter, r *http.Request, bucket string, fsys fs.FS) {
	if r.PathValue("bucket") != bucket {
		http.NotFound(w, r)
		return
	}
	objName := r.PathValue("object")
	f, err := fsys.Open(objName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			http.NotFound(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if !info.ModTime().IsZero() {
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	}
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}
