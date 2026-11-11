// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gcsfs provides a simple [io/fs.FS] implementation for Google Cloud Storage.
package gcsfs

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// FS is an [fs.ReadDirFS] backed by a GCS bucket.
// It implements only what is needed for reading download data.
type FS struct {
	ctx    context.Context
	client *storage.Client
	bucket *storage.BucketHandle
}

var _ fs.ReadDirFS = (*FS)(nil)

// New creates a new FS for the given GCS bucket name.
// The provided ctx governs the lifetime of all subsequent Open and ReadDir calls
// (and readers returned by Open); callers should not cancel it while the FS is in use.
func New(ctx context.Context, bucketName string) (*FS, error) {
	if bucketName == "" {
		return nil, errors.New("gcsfs.New: empty bucket name")
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &FS{
		ctx:    ctx,
		client: client,
		bucket: client.Bucket(bucketName),
	}, nil
}

// Close closes the underlying GCS client.
func (f *FS) Close() error {
	return f.client.Close()
}

// Open opens the named file for reading.
// It supports only files, not directories, so fs.Stat on directories,
// fs.WalkDir, and fstest.TestFS are not supported.
func (f *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	r, err := f.bucket.Object(name).NewReader(f.ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			err = fs.ErrNotExist
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &file{name: path.Base(name), Reader: r}, nil
}

// ReadDir reads the named directory and returns a list of directory entries
// sorted by filename.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	prefix := ""
	if name != "." {
		// Trailing slash ensures we only match objects inside this directory
		// rather than sibling directories sharing the same string prefix.
		prefix = name + "/"
	}
	it := f.bucket.Objects(f.ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: "/",
	})
	var entries []fs.DirEntry
	sawDirMarker := false
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
		}
		if attrs.Prefix != "" {
			sub := strings.TrimSuffix(strings.TrimPrefix(attrs.Prefix, prefix), "/")
			if sub != "" {
				entries = append(entries, fs.FileInfoToDirEntry(&fileInfo{
					name:  sub,
					isDir: true,
				}))
			}
			continue
		}
		// Tools like gcloud and the Cloud Console represent empty folders as
		// zero-byte marker objects named "dir/". Record the marker so an
		// otherwise empty directory returns an empty slice instead of
		// fs.ErrNotExist.
		if attrs.Name == prefix {
			sawDirMarker = true
			continue
		}
		if attrs.Name != "" {
			entries = append(entries, fs.FileInfoToDirEntry(&fileInfo{
				name:    strings.TrimPrefix(attrs.Name, prefix),
				size:    attrs.Size,
				modTime: attrs.Updated,
			}))
		}
	}
	if name != "." && len(entries) == 0 && !sawDirMarker {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

type file struct {
	name string
	*storage.Reader
}

func (f *file) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    f.name,
		size:    f.Reader.Attrs.Size,
		modTime: f.Reader.Attrs.LastModified,
	}, nil
}

type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string { return fi.name }
func (fi *fileInfo) Size() int64  { return fi.size }
func (fi *fileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0555
	}
	return 0444
}
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() any           { return nil }
