// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/osv"
)

type source interface {
	// get returns the raw, uncompressed bytes at the
	// requested endpoint, which should be bare with no file extensions
	// (e.g., "index/modules" instead of "index/modules.json.gz").
	// It errors if the endpoint cannot be reached or does not exist
	// in the expected form.
	get(ctx context.Context, endpoint string) ([]byte, error)
}

// NewSource returns a source interface from src, which must be a URL with one of
// the schemes "file", http", or "https".
// It returns an error if the given url is invalid or does not exist.
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

func (hs *httpSource) get(ctx context.Context, endpoint string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "vuln.httpSource.get(%q)", endpoint)

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

// Create a new in-memory source for testing.
// Adapted from x/vulndb/internal/database.go.
func newInMemorySource(entries []*osv.Entry) (*inMemorySource, error) {
	data := make(map[string][]byte)
	db := DBMeta{}
	modulesMap := make(map[string]*ModuleMeta)
	vulnsMap := make(map[string]*VulnMeta)
	for _, entry := range entries {
		if entry.ID == "" {
			return nil, fmt.Errorf("entry %v has no ID", entry)
		}
		if _, ok := vulnsMap[entry.ID]; ok {
			return nil, fmt.Errorf("id %q appears twice", entry.ID)
		}
		if entry.Modified.After(db.Modified) {
			db.Modified = entry.Modified
		}
		for _, affected := range entry.Affected {
			modulePath := affected.Module.Path
			if _, ok := modulesMap[modulePath]; !ok {
				modulesMap[modulePath] = &ModuleMeta{
					Path:  modulePath,
					Vulns: []ModuleVuln{},
				}
			}
			module := modulesMap[modulePath]
			module.Vulns = append(module.Vulns, ModuleVuln{
				ID:       entry.ID,
				Modified: entry.Modified,
				Fixed:    osv.LatestFixedVersion(affected.Ranges),
			})
		}
		vulnsMap[entry.ID] = &VulnMeta{
			ID:       entry.ID,
			Modified: entry.Modified,
			Aliases:  entry.Aliases,
		}
		b, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		data[idDir+"/"+entry.ID] = b
	}

	b, err := json.Marshal(db)
	if err != nil {
		return nil, err
	}
	data[dbEndpoint] = b

	// Add the modules endpoint.
	modules := make([]*ModuleMeta, 0, len(modulesMap))
	for _, module := range modulesMap {
		modules = append(modules, module)
	}
	sort.SliceStable(modules, func(i, j int) bool {
		return modules[i].Path < modules[j].Path
	})
	for _, module := range modules {
		sort.SliceStable(module.Vulns, func(i, j int) bool {
			return module.Vulns[i].ID < module.Vulns[j].ID
		})
	}
	b, err = json.Marshal(modules)
	if err != nil {
		return nil, err
	}
	data[modulesEndpoint] = b

	// Add the vulns endpoint.
	vulns := make([]*VulnMeta, 0, len(vulnsMap))
	for _, vuln := range vulnsMap {
		vulns = append(vulns, vuln)
	}
	sort.SliceStable(vulns, func(i, j int) bool {
		return vulns[i].ID < vulns[j].ID
	})
	b, err = json.Marshal(vulns)
	if err != nil {
		return nil, err
	}
	data[vulnsEndpoint] = b

	return &inMemorySource{data: data}, nil
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
