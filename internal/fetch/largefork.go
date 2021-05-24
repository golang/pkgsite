// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

// Recognizing and skipping forks of large module versions that don't have a go.mod file.

//go:generate go run gen_zip_signatures.go -v

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Modver is a module path and version.
type Modver struct {
	ModulePath string
	Version    string
}

func (m Modver) String() string {
	return m.ModulePath + "@" + m.Version
}

// ZipSignature calculates a signature that uniquely identifies a zip file.
// It hashes every filename and its contents. Filenames must begin with prefix,
// which is not included in the hash.
func ZipSignature(r *zip.Reader, prefix string) (string, error) {
	files := make([]*zip.File, len(r.File))
	copy(files, r.File)
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	h := sha256.New()
	for _, f := range files {
		if !strings.HasPrefix(f.Name, prefix) {
			return "", fmt.Errorf("zip file %q does not have prefix %q", f.Name, prefix)
		}
		io.WriteString(h, f.Name[len(prefix):])
		h.Write([]byte{0})
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		io.Copy(h, rc)
		rc.Close()
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// forkedFrom returns a module that the current one has been forked from. It
// consults a built-in list of modules and their zip signatures, and returns a
// module path from that list if its zip file and version are identical to the
// given ones. If there is no matching module, it returns the empty string.
func forkedFrom(z *zip.Reader, module, version string) (string, error) {
	sig, err := ZipSignature(z, module+"@"+version)
	if err != nil {
		return "", err
	}
	for _, mv := range ZipSignatures[sig] {
		if mv.ModulePath != module && mv.Version == version {
			return mv.ModulePath, nil
		}
	}
	// Either signature or version didn't match.
	return "", nil
}
