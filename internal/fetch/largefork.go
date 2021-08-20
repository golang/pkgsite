// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

// Recognizing and skipping forks of large module versions that don't have a go.mod file.

//go:generate go run gen_zip_signatures.go -v

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
)

// FSSignature calculates a signature that uniquely identifies a filesystem.
// It hashes every filename and its contents.
func FSSignature(fsys fs.FS) (string, error) {
	// To match the behavior of the old ZipSignatures function that this is
	// based on, sort the paths from fs.WalkDir. Although fs.WalkDir traverses
	// the files in lexical order within each directory, that is not the same
	// order as sorting all the paths. For example, fs.WalkDir will return
	// ["a/b", "a#b"], but because '#' comes before '/', sorting the paths
	// swaps them.
	var paths []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // we can get NotExist on an empty FS
		return "", err
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, path := range paths {
		io.WriteString(h, "/"+path) // slash needed to match ZipSignatures
		h.Write([]byte{0})
		rc, err := fsys.Open(path)
		if err != nil {
			return "", err
		}
		io.Copy(h, rc)
		rc.Close()
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// forkedFrom returns a module that the current one has been forked from. It
// consults a built-in list of modules and their signatures, and returns a
// module path from that list if its contents and version are identical to the
// given ones. If there is no matching module, it returns the empty string.
func forkedFrom(moduleContents fs.FS, module, version string) (string, error) {
	sig, err := FSSignature(moduleContents)
	if err != nil {
		return "", err
	}
	for _, mv := range ZipSignatures[sig] {
		if mv.Path != module && mv.Version == version {
			return mv.Path, nil
		}
	}
	// Either signature or version didn't match.
	return "", nil
}
