// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSource(t *testing.T) {
	t.Run("https", func(t *testing.T) {
		url := "https://vuln.go.dev"
		s, err := NewSource(url)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.(*httpSource); !ok {
			t.Errorf("NewSource(%s) = %#v, want type *httpSource ", url, s)
		}
	})

	t.Run("file", func(t *testing.T) {
		fileURL := "file:///" + t.TempDir()
		s, err := NewSource(fileURL)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.(*localSource); !ok {
			t.Errorf("NewSource(%s) = %#v, want type *localSource", fileURL, s)
		}
	})
}

func TestHTTPSource(t *testing.T) {
	want := []byte("some data")
	gzipped, err := gzipped(want)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/test/endpoint.json.gz" {
			if _, err := rw.Write(gzipped); err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	src := httpSource{
		url: server.URL,
		c:   server.Client(),
	}
	got, err := src.get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("httpSource.get = %s, want %s", got, want)
	}
}

func TestLocalSource(t *testing.T) {
	temp := t.TempDir()
	if err := os.Mkdir(filepath.Join(temp, "test"), 0755); err != nil {
		t.Fatal(err)
	}

	want := []byte("some data")
	if err := os.WriteFile(filepath.Join(temp, "test/endpoint.json"), want, 0644); err != nil {
		t.Fatal(err)
	}

	src := localSource{
		dir: temp,
	}
	got, err := src.get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("localSource.get = %s, want %s", got, want)
	}
}

func TestInMemorySource(t *testing.T) {
	want := []byte("some data")
	src := inMemorySource{
		data: map[string][]byte{
			"test/endpoint": want,
		},
	}

	got, err := src.get(context.Background(), "test/endpoint")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("inMemorySource.get = %s, want %s", got, want)
	}
}

func gzipped(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	defer w.Close()
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
