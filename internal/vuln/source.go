// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type source interface {
	// get returns the raw, uncompressed bytes at the
	// requested endpoint, which should be bare with no file extensions
	// (e.g., "index/modules" instead of "index/modules.json.gz").
	// It errors if the endpoint cannot be reached or does not exist
	// in the expected form.
	get(ctx context.Context, endpoint string) ([]byte, error)
}

// NewSource returns a source interface from a http:// or file:// prefixed
// url src. It errors if the given url is invalid or does not exist.
func NewSource(src string) (source, error) {
	uri, err := url.Parse(src)
	if err != nil {
		return nil, err
	}
	switch uri.Scheme {
	case "http", "https":
		return &httpSource{url: uri.String(), c: http.DefaultClient}, nil
	case "file":
		dir, err := URLToFilePath(uri)
		if err != nil {
			return nil, err
		}
		fi, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", dir)
		}
		return &localSource{dir: dir}, nil
	default:
		return nil, fmt.Errorf("src %q has unsupported scheme", uri)
	}
}

// httpSource reads databases from an http(s) source.
// Intended for use in production.
type httpSource struct {
	url string
	c   *http.Client
}

func (hs *httpSource) get(ctx context.Context, endpoint string) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/%s", hs.url, endpoint+".json.gz")
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hs.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status code: %d", req.URL, resp.StatusCode)
	}

	// Uncompress the result.
	r, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// localSource reads databases from a local directory.
// Intended for use in unit tests and screentests.
type localSource struct {
	dir string
}

func (db *localSource) get(ctx context.Context, endpoint string) ([]byte, error) {
	return os.ReadFile(filepath.Join(db.dir, endpoint+".json"))
}

// inMemorySource reads databases from an in-memory map.
// Intended for use in unit tests.
type inMemorySource struct {
	data map[string][]byte
}

func (db *inMemorySource) get(ctx context.Context, endpoint string) ([]byte, error) {
	b, ok := db.data[endpoint]
	if !ok {
		return nil, fmt.Errorf("no data found at endpoint %q", endpoint)
	}
	return b, nil
}
