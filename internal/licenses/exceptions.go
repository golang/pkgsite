// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"fmt"
	"reflect"
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
	unmatched := map[string]bool{} // set of files not yet seen
	for name := range matchFiles {
		unmatched[name] = true
	}
	for _, f := range d.zr.File {
		pathname := strings.TrimPrefix(f.Name, prefix)
		if mf, ok := matchFiles[pathname]; ok {
			data, err := readZipFile(f)
			if err != nil {
				d.logf("reading zip file %s: %v", f.Name, err)
				return false, nil
			}
			got := strings.Fields(string(data))
			want := strings.Fields(mf.contents)
			if !reflect.DeepEqual(got, want) {
				d.logf("isException(%q, %q): contents of %q do not match", d.modulePath, d.version, pathname)
				return false, nil
			}
			delete(unmatched, pathname)
		}
	}
	if len(unmatched) > 0 {
		d.logf("isException(%q, %q): unmatched files: %s",
			d.modulePath, d.version, strings.Join(setToSortedSlice(unmatched), ", "))
		return false, nil
	}
	// There can be no other license files in the zip.
	files := d.Files(AllFiles)
	others := map[string]bool{}
	for _, f := range files {
		pathname := strings.TrimPrefix(f.Name, prefix)
		if _, ok := matchFiles[pathname]; !ok {
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
				Contents: []byte(mf.contents),
			})
		}
	}
	return true, lics
}

// A matchFile is a file whose contents must be matched in
// order for a special-cased module to be considered redistributable.
type matchFile struct {
	types    []string // license types, or nil if not a license
	contents string
}

// A map of exceptions to our license policy. The keys are module paths. The
// values are a set of file paths and their contents that must match
// word-for-word (i.e. ignoring all whitespace), along with a set of types to
// return on a match. The license files must all be at top level.
//


var exceptions = map[string]map[string]matchFile{
	"gioui.org": map[string]matchFile{
		"COPYING": matchFile{
			nil,
			`This project is dual-licensed under the UNLICENSE (see UNLICENSE) or
			the MIT license (see LICENSE-MIT). You may use the project under the terms
			of either license.`,
		},
		"LICENSE-MIT": matchFile{
			[]string{"MIT"},
			`
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
			THE SOFTWARE.`,
		},
		"UNLICENSE": matchFile{
			[]string{"Unlicense"},
			`
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

			For more information, please refer to <https://unlicense.org/>`,
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
