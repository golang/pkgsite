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

// The license used by Canonical is a LGPL-3.0 license with some additional text prepended.

var canonicalLicense = `
			This software is licensed under the LGPLv3, included below.

			As a special exception to the GNU Lesser General Public License version 3
			("LGPL3"), the copyright holders of this Library give you permission to
			convey to a third party a Combined Work that links statically or dynamically
			to this Library without providing any Minimal Corresponding Source or
			Minimal Application Code as set out in 4d or providing the installation
			information set out in section 4e, provided that you comply with the other
			provisions of LGPL3 and provided that you meet, for the Application the
			terms and conditions of the license(s) which apply to the Application.

			Except as stated in this special exception, the provisions of LGPL3 will
			continue to comply in full to this Library. If you modify this Library, you
			may apply this exception to your version of this Library, but you are not
			obliged to do so. If you do not wish to do so, delete this exception
			statement from your version. This exception does not (and cannot) modify any
			license terms which apply to the Application, with which you must still
			comply.


							   GNU LESSER GENERAL PUBLIC LICENSE
								   Version 3, 29 June 2007

			 Copyright (C) 2007 Free Software Foundation, Inc. <http://fsf.org/>
			 Everyone is permitted to copy and distribute verbatim copies
			 of this license document, but changing it is not allowed.


			  This version of the GNU Lesser General Public License incorporates
			the terms and conditions of version 3 of the GNU General Public
			License, supplemented by the additional permissions listed below.

			  0. Additional Definitions.

			  As used herein, "this License" refers to version 3 of the GNU Lesser
			General Public License, and the "GNU GPL" refers to version 3 of the GNU
			General Public License.

			  "The Library" refers to a covered work governed by this License,
			other than an Application or a Combined Work as defined below.

			  An "Application" is any work that makes use of an interface provided
			by the Library, but which is not otherwise based on the Library.
			Defining a subclass of a class defined by the Library is deemed a mode
			of using an interface provided by the Library.

			  A "Combined Work" is a work produced by combining or linking an
			Application with the Library.  The particular version of the Library
			with which the Combined Work was made is also called the "Linked
			Version".

			  The "Minimal Corresponding Source" for a Combined Work means the
			Corresponding Source for the Combined Work, excluding any source code
			for portions of the Combined Work that, considered in isolation, are
			based on the Application, and not on the Linked Version.

			  The "Corresponding Application Code" for a Combined Work means the
			object code and/or source code for the Application, including any data
			and utility programs needed for reproducing the Combined Work from the
			Application, but excluding the System Libraries of the Combined Work.

			  1. Exception to Section 3 of the GNU GPL.

			  You may convey a covered work under sections 3 and 4 of this License
			without being bound by section 3 of the GNU GPL.

			  2. Conveying Modified Versions.

			  If you modify a copy of the Library, and, in your modifications, a
			facility refers to a function or data to be supplied by an Application
			that uses the facility (other than as an argument passed when the
			facility is invoked), then you may convey a copy of the modified
			version:

			   a) under this License, provided that you make a good faith effort to
			   ensure that, in the event an Application does not supply the
			   function or data, the facility still operates, and performs
			   whatever part of its purpose remains meaningful, or

			   b) under the GNU GPL, with none of the additional permissions of
			   this License applicable to that copy.

			  3. Object Code Incorporating Material from Library Header Files.

			  The object code form of an Application may incorporate material from
			a header file that is part of the Library.  You may convey such object
			code under terms of your choice, provided that, if the incorporated
			material is not limited to numerical parameters, data structure
			layouts and accessors, or small macros, inline functions and templates
			(ten or fewer lines in length), you do both of the following:

			   a) Give prominent notice with each copy of the object code that the
			   Library is used in it and that the Library and its use are
			   covered by this License.

			   b) Accompany the object code with a copy of the GNU GPL and this license
			   document.

			  4. Combined Works.

			  You may convey a Combined Work under terms of your choice that,
			taken together, effectively do not restrict modification of the
			portions of the Library contained in the Combined Work and reverse
			engineering for debugging such modifications, if you also do each of
			the following:

			   a) Give prominent notice with each copy of the Combined Work that
			   the Library is used in it and that the Library and its use are
			   covered by this License.

			   b) Accompany the Combined Work with a copy of the GNU GPL and this license
			   document.

			   c) For a Combined Work that displays copyright notices during
			   execution, include the copyright notice for the Library among
			   these notices, as well as a reference directing the user to the
			   copies of the GNU GPL and this license document.

			   d) Do one of the following:

				   0) Convey the Minimal Corresponding Source under the terms of this
				   License, and the Corresponding Application Code in a form
				   suitable for, and under terms that permit, the user to
				   recombine or relink the Application with a modified version of
				   the Linked Version to produce a modified Combined Work, in the
				   manner specified by section 6 of the GNU GPL for conveying
				   Corresponding Source.

				   1) Use a suitable shared library mechanism for linking with the
				   Library.  A suitable mechanism is one that (a) uses at run time
				   a copy of the Library already present on the user's computer
				   system, and (b) will operate properly with a modified version
				   of the Library that is interface-compatible with the Linked
				   Version.

			   e) Provide Installation Information, but only if you would otherwise
			   be required to provide such information under section 6 of the
			   GNU GPL, and only to the extent that such information is
			   necessary to install and execute a modified version of the
			   Combined Work produced by recombining or relinking the
			   Application with a modified version of the Linked Version. (If
			   you use option 4d0, the Installation Information must accompany
			   the Minimal Corresponding Source and Corresponding Application
			   Code. If you use option 4d1, you must provide the Installation
			   Information in the manner specified by section 6 of the GNU GPL
			   for conveying Corresponding Source.)

			  5. Combined Libraries.

			  You may place library facilities that are a work based on the
			Library side by side in a single library together with other library
			facilities that are not Applications and are not covered by this
			License, and convey such a combined library under terms of your
			choice, if you do both of the following:

			   a) Accompany the combined library with a copy of the same work based
			   on the Library, uncombined with any other library facilities,
			   conveyed under the terms of this License.

			   b) Give prominent notice with the combined library that part of it
			   is a work based on the Library, and explaining where to find the
			   accompanying uncombined form of the same work.

			  6. Revised Versions of the GNU Lesser General Public License.

			  The Free Software Foundation may publish revised and/or new versions
			of the GNU Lesser General Public License from time to time. Such new
			versions will be similar in spirit to the present version, but may
			differ in detail to address new problems or concerns.

			  Each version is given a distinguishing version number. If the
			Library as you received it specifies that a certain numbered version
			of the GNU Lesser General Public License "or any later version"
			applies to it, you have the option of following the terms and
			conditions either of that published version or of any later version
			published by the Free Software Foundation. If the Library as you
			received it does not specify a version number of the GNU Lesser
			General Public License, you may choose any version of the GNU Lesser
			General Public License ever published by the Free Software Foundation.

			  If the Library as you received it specifies that a proxy can decide
			whether future versions of the GNU Lesser General Public License shall
			apply, that proxy's public statement of acceptance of any version is
			permanent authorization for you to choose that version for the
			Library.`

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
	{
		types:    []string{"LGPL-3.0"},
		contents: normalize(canonicalLicense),
	},
	{
		// The Canonical license often occurs with a short prefix. Since our exception
		// mechanism doesn't do partial matching, we include it here.
		types: []string{"LGPL-3.0"},
		contents: normalize(`All files in this repository are licensed as follows. If you contribute
			to this repository, it is assumed that you license your contribution
			under the same license unless you state otherwise.
			All files Copyright (C) 2015 Canonical Ltd. unless otherwise specified in the file. ` +
			canonicalLicense),
	},
}
