// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// isException reports whether a module is an exception to the rules for
// determining redistributability. There is a fixed list of such modules, along
// with the files in the module that must match what we have stored. In
// addition, there can be no other license files anywhere in the module.
//
// If isException returns true, the module and all the packages it contains are
// redistributable, and its second return value is the list of module licenses.
// If it returns false, neither the module nor its packages are redistributable.
func (d *Detector) isException() (bool, []*License) {
	prefix := pathPrefix(contentsDir(d.modulePath, d.version))
	matchFiles := exceptions[d.modulePath]
	if matchFiles == nil {
		return false, nil
	}
	matchedContents := map[string][]byte{} // contents of files that match
	for _, f := range d.zr.File {
		pathname := strings.TrimPrefix(f.Name, prefix)
		if mf, ok := matchFiles[pathname]; ok {
			data, err := readZipFile(f)
			if err != nil {
				d.logf("reading zip file %s: %v", f.Name, err)
				return false, nil
			}
			if normalize(string(data)) != mf.contents {
				d.logf("isException(%q, %q): contents of %q do not match", d.modulePath, d.version, pathname)
				return false, nil
			}
			matchedContents[pathname] = data
		}
	}

	// Check if any files were not matched.
	var unmatched []string
	for name := range matchFiles {
		if matchedContents[name] == nil {
			unmatched = append(unmatched, name)
		}
	}
	if len(unmatched) > 0 {
		sort.Strings(unmatched)
		d.logf("isException(%q, %q): unmatched files: %s", d.modulePath, d.version, strings.Join(unmatched, ", "))
		return false, nil
	}

	// There can be no other license files in the zip, unless they are in a testdata directory.
	files := d.Files(AllFiles)
	others := map[string]bool{}
	for _, f := range files {
		pathname := strings.TrimPrefix(f.Name, prefix)
		if _, ok := matchFiles[pathname]; !ok && path.Base(path.Dir(pathname)) != "testdata" {
			others[pathname] = true
		}
	}
	if len(others) > 0 {
		d.logf("isException(%q, %q): other license files: %s",
			d.modulePath, d.version, strings.Join(setToSortedSlice(others), ", "))
		return false, nil
	}

	// Success; gather all license files so the discovery site can store and show them.
	var lics []*License
	for pathname, mf := range matchFiles {
		if mf.types != nil {
			lics = append(lics, &License{
				Metadata: &Metadata{
					Types:    mf.types,
					FilePath: pathname,
				},
				Contents: matchedContents[pathname],
			})
		}
	}
	sort.Slice(lics, func(i, j int) bool { return lics[i].FilePath < lics[j].FilePath })
	return true, lics
}

// normalize produces a normalized version of s, for looser comparison.
// It lower-cases the string and replaces all consecutive whitespace with
// a single space.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// A matchFile is a file whose contents must be matched in
// order for a special-cased module to be considered redistributable.
type matchFile struct {
	types    []string // license types, or nil if not a license
	contents string   // normalized
}

// A map of exceptions to our license policy. The keys are module paths. The
// values are a set of file paths and their contents that must match
// word-for-word (i.e. ignoring all whitespace), along with a set of types to
// return on a match. The license files must all be at top level.
//


var exceptions = map[string]map[string]*matchFile{
	"gioui.org": map[string]*matchFile{
		"COPYING": &matchFile{
			types: nil,
			contents: normalize(`This project is dual-licensed under the UNLICENSE (see UNLICENSE) or
			the MIT license (see LICENSE-MIT). You may use the project under the terms
			of either license.`),
		},
		"LICENSE-MIT": &matchFile{
			types: []string{"MIT"},
			contents: normalize(`
			The MIT License (MIT)

			Copyright (c) 2019 The Gio authors

			Permission is hereby granted, free of charge, to any person obtaining a copy
			of this software and associated documentation files (the "Software"), to deal
			in the Software without restriction, including without limitation the rights
			to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
			copies of the Software, and to permit persons to whom the Software is
			furnished to do so, subject to the following conditions:

			The above copyright notice and this permission notice shall be included in
			all copies or substantial portions of the Software.

			THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
			IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
			FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
			AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
			LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
			OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
			THE SOFTWARE.`),
		},
		"UNLICENSE": &matchFile{
			types: []string{"Unlicense"},
			contents: normalize(`
			This is free and unencumbered software released into the public domain.

			Anyone is free to copy, modify, publish, use, compile, sell, or
			distribute this software, either in source code form or as a compiled
			binary, for any purpose, commercial or non-commercial, and by any
			means.

			In jurisdictions that recognize copyright laws, the author or authors
			of this software dedicate any and all copyright interest in the
			software to the public domain. We make this dedication for the benefit
			of the public at large and to the detriment of our heirs and
			successors. We intend this dedication to be an overt act of
			relinquishment in perpetuity of all present and future rights to this
			software under copyright law.

			THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
			EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
			MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
			IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM, DAMAGES OR
			OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
			ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
			OTHER DEALINGS IN THE SOFTWARE.

			For more information, please refer to <https://unlicense.org/>`),
		},
	},
}

func init() {
	for _, matchFiles := range exceptions {
		for filename := range matchFiles {
			if strings.ContainsRune(filename, '/') {
				panic(fmt.Sprintf("exception filename %q not at top level", filename))
			}
		}
	}
}

// exceptionFileTypes returns the license types of the file with contents if it
// is in the list of exceptions, with a second return value of true. Otherwise
// it returns nil,  false.
func exceptionFileTypes(contents []byte) ([]string, bool) {
	norm := normalize(string(contents))
	for _, mf := range exceptionFiles {
		if norm == mf.contents {
			return mf.types, true
		}
	}
	return nil, false
}

var exceptionFiles = []*matchFile{
	{
		types: []string{"BSD-3-Clause"},
		contents: normalize(`
			Copyright ©2013 The Gonum Authors. All rights reserved.

			Redistribution and use in source and binary forms, with or without
			modification, are permitted provided that the following conditions are met:
				* Redistributions of source code must retain the above copyright
				  notice, this list of conditions and the following disclaimer.
				* Redistributions in binary form must reproduce the above copyright
				  notice, this list of conditions and the following disclaimer in the
				  documentation and/or other materials provided with the distribution.
				* Neither the name of the gonum project nor the names of its authors and
				  contributors may be used to endorse or promote products derived from this
				  software without specific prior written permission.

			THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
			ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
			WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
			DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
			FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
			DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
			SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
			CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
			OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
			OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.`),
	},
}
