// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/google/licensecheck"
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

// Additional licenses that licensecheck perhaps should recognize. These are
// here to work around regressions in our database. They are hopefully
// temporary. For each, either licensecheck should recognize it (not likely,
// since the extra text is really not license text), or we should handle the
// case more generally, or maybe we should ask the author to help us.
var extraLicenses = []licensecheck.License{
	{
		// From https://github.com/sgmitchell/atlantis/blob/v0.9.0/LICENSE.
		Name: "Apache-2.0",
		Text: `
Atlantis was originally copyrighted and licensed under:

    Copyright 2017 HootSuite Media Inc.

    Licensed under the Apache License, Version 2.0 (the "License");
    you may not use this file except in compliance with the License.
    You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.

In 2018 it was forked from github.com/hootsuite/atlantis to
github.com/runatlantis/atlantis. The contents of files created before the fork
are obviously still under the Hootsuite copyright and contain a header to that
effect in addition to a disclaimer that they have subsequently been modified by
contributors to github.com/runatlantis/atlantis. Modifications and new files
hereafter are still under the Apache 2.0 license, but are not under copyright of
Hootsuite Media Inc.`,
	},
	{
		// From https://github.com/Workiva/frugal/blob/master/LICENSE.
		Name: "Apache-2.0",
		Text: `
Copyright 2017-2018 Workiva Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

--------------------------------------------------

This Frugal software includes a number of subcomponents with
separate copyright notices and/or license terms. Your use of the source
code for the these subcomponents is subject to the terms and
conditions of the following licenses:

Apache Thrift software: https://github.com/apache/thrift
Copyright ©2017 Apache Software Foundation.
Licensed under the Apache License v2.0: https://github.com/apache/thrift/blob/master/LICENSE
Apache, Apache Thrift, and the Apache feather logo are trademarks of The Apache Software Foundation.

--------------------------------------------------`,
	},
	{
		// From https://github.com/gardener/autoscaler/blob/v0.1.0/LICENSE.
		Name: "Apache-2.0",
		Text: `
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "[]"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright [yyyy] [name of copyright owner]

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

## APIs

This project may include APIs to SAP or third party products or services. The use of these APIs, products and services may be subject to additional agreements. In no event shall the application of the Apache Software License, v.2 to this project grant any rights in or to these APIs, products or services that would alter, expand, be inconsistent with, or supersede any terms of these additional agreements. API means application programming interfaces, as well as their respective specifications and implementing code that allows other software products to communicate with or call on SAP or third party products or services (for example, SAP Enterprise Services, BAPIs, Idocs, RFCs and ABAP calls or other user exits) and may be made available through SAP or third party products, SDKs, documentation or other media.

## Subcomponents

This project includes the following subcomponents that are subject to separate license terms.
Your use of these subcomponents is subject to the separate license terms applicable to
each subcomponent.

Set of dependencies used by [kubernetes/autoscaler](https://github.wdf.sap.corp/kubernetes/autoscaler/) can be found
at [Godeps.json](./cluster-autoscaler/Godeps/Godeps.json).

Google Cloud Client Libraries for Go
https://github.com/GoogleCloudPlatform/google-cloud-go
Copyright 2014 Google Inc.
Apache 2 license https://github.com/GoogleCloudPlatform/google-cloud-go/blob/master/LICENSE

Microsoft Azure SDK for Go
https://github.com/Azure/azure-sdk-for-go
Copyright 2016 Microsoft Corporation
Apache 2 license https://github.com/Azure/azure-sdk-for-go/blob/master/LICENSE

Go package for ANSI terminal emulation in Windows
https://github.com/Azure/go-ansiterm
Copyright (c) 2015 Microsoft Corporation
MIT license https://github.com/Azure/go-ansiterm/blob/master/LICENSE

https://github.com/Azure/go-ansiterm
Copyright (c) 2015 Microsoft Corporation
MIT license https://github.com/Azure/go-ansiterm/blob/master/LICENSE

Windows Performance Data Helper wrapper package for Go
https://github.com/JeffAshton/win_pdh/
Copyright (c) 2010 The win_pdh Authors. All rights reserved.
3 clause BSD license https://github.com/JeffAshton/win_pdh/blob/master/LICENSE

Package heredoc provides the here-document with keeping indent.
https://github.com/MakeNowJust/heredoc
Copyright (c) 2014-2017 TSUYUSATO Kitsune
MIT license https://github.com/MakeNowJust/heredoc/blob/master/LICENSE

Win32 IO-related utilities for Go
https://github.com/Microsoft/go-winio
Copyright (c) 2015 Microsoft
MIT license https://github.com/Microsoft/go-winio/blob/master/LICENSE

Windows - Host Compute Service Shim
https://github.com/Microsoft/hcsshim
Copyright (c) 2015 Microsoft
MIT license https://github.com/Microsoft/hcsshim/blob/master/LICENSE

Golang middleware to gzip HTTP responses
https://github.com/NYTimes/gziphandler
Copyright 2016-2017 The New York Times Company
Apache 2 license https://github.com/NYTimes/gziphandler/blob/master/LICENSE

Gotty is a library written in Go that provides interpretation and loading of Termcap database files.
https://github.com/Nvveen/Gotty
Copyright (c) 2012, Neal van Veen (nealvanveen@gmail.com)
2 clause BSD license https://github.com/Nvveen/Gotty/blob/master/LICENSE

purell - tiny Go library to normalize URLs
https://github.com/PuerkitoBio/purell
Copyright (c) 2012, Martin Angers
3 clause BSD license https://github.com/PuerkitoBio/purell/blob/master/LICENSE

urlesc - Proper URL escaping as per RFC3986
https://github.com/PuerkitoBio/urlesc
Copyright (c) 2012 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/PuerkitoBio/urlesc/blob/master/LICENSE

go-http-auth - Basic and Digest HTTP Authentication for golang http
https://github.com/abbot/go-http-auth
Apache 2 license https://github.com/abbot/go-http-auth/blob/master/LICENSE

appc spec - App Container Specification and Tooling
https://github.com/appc/spec
Copyright 2015 The appc Authors
Apache 2 license https://github.com/appc/spec/blob/master/LICENSE

circbuf - Golang circular (ring) buffer
https://github.com/armon/circbuf
Copyright (c) 2013 Armon Dadgar
MIT license https://github.com/armon/circbuf/blob/master/LICENSE

AWS SDK for the Go programming language.
https://github.com/aws/aws-sdk-go
Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
Copyright 2014-2015 Stripe, Inc.
Apache 2 license https://github.com/aws/aws-sdk-go/blob/master/LICENSE.txt

perks - Effective Computation of Things
https://github.com/beorn7/perks
Copyright (C) 2013 Blake Mizerany
MIT license https://github.com/beorn7/perks/blob/master/LICENSE

semver - Semantic Versioning (semver) library written in golang
https://github.com/blang/semver
Copyright (c) 2014 Benedikt Lang <github at benediktlang.de>
MIT license https://github.com/blang/semver/blob/master/LICENSE

Flocker go library
https://github.com/clusterhq/flocker-go
Copyright 2014-2016 ClusterHQ
Apache 2 license https://github.com/ClusterHQ/flocker-go/blob/master/LICENSE

goscaleio - ScaleIO package that provides API bindings for Go
https://github.com/thecodeteam/goscaleio
Apache 2 license https://github.com/thecodeteam/goscaleio/blob/master/LICENSE

Container Storage Interface (CSI) Specification.
https://github.com/container-storage-interface/spec
Apache 2 license https://github.com/container-storage-interface/spec/blob/master/LICENSE

containerd console - console package for Go
https://github.com/containerd/console
Copyright The containerd Authors.
Apache 2 license https://github.com/containerd/console/blob/master/LICENSE

containerd - An open and reliable container runtime
https://github.com/containerd/containerd
Copyright The containerd Authors.
Apache 2 license https://github.com/containerd/containerd/blob/master/LICENSE

Container Network Interface - networking for Linux containers
https://github.com/containernetworking/cni
Copyright 2016 CNI authors
Apache 2 license https://github.com/containernetworking/cni/blob/master/LICENSE

etcd - Distributed reliable key-value store for the most critical data of a distributed system
https://github.com/coreos/etcd
Copyright 2014 CoreOS, Inc
Apache 2 license https://github.com/coreos/etcd/blob/master/LICENSE

go-semver - semver library in Go
https://github.com/coreos/go-semver
Copyright 2018 CoreOS, Inc
Apache 2 license https://github.com/coreos/go-semver/blob/master/LICENSE

go-systemd - Go bindings to systemd socket activation, journal, D-Bus, and unit files
https://github.com/coreos/go-systemd
Copyright 2018 CoreOS, Inc
Apache 2 license https://github.com/coreos/go-systemd/blob/master/LICENSE

coreos pkg - a collection of go utility packages
https://github.com/coreos/pkg
Copyright 2014 CoreOS, Inc
Apache 2 license https://github.com/coreos/pkg/blob/master/LICENSE

rkt - is a pod-native container engine for Linux.
https://github.com/rkt/rkt
Copyright 2016 The rkt Authors
Apache 2 license https://github.com/rkt/rkt/blob/master/LICENSE

cyphar filepath-securejoin - Proposed filepath.SecureJoin implementation
https://github.com/cyphar/filepath-securejoin
Copyright (C) 2014-2015 Docker Inc & Go Authors. All rights reserved.
Copyright (C) 2017 SUSE LLC. All rights reserved.
3 clause BSD license https://github.com/cyphar/filepath-securejoin/blob/master/LICENSE

DHCP4 library written in Go.
https://github.com/d2g/dhcp4
Copyright (c) 2013 Skagerrak Software Limited. All rights reserved.
3 clause BSD license https://github.com/d2g/dhcp4/blob/master/LICENSE

dhcp4client - DHCP Client
https://github.com/d2g/dhcp4client
Mozilla Public License, version 2.0

go-spew - Implements a deep pretty printer for Go data structures to aid in debugging
https://github.com/davecgh/go-spew
Copyright (c) 2012-2016 Dave Collins <dave@davec.name>
ISC License

safefile - Go package safefile implements safe "atomic" saving of files.
https://github.com/dchest/safefile
Copyright (c) 2013 Dmitry Chestnykh <dmitry@codingrobots.com>
2 clause BSD license https://github.com/dchest/safefile/blob/master/LICENSE

jwdt-go - Golang implementation of JSON Web Tokens (JWT)
https://github.com/dgrijalva/jwt-go
Copyright (c) 2012 Dave Grijalva
MIT license https://github.com/dgrijalva/jwt-go/blob/master/LICENSE

The Docker toolset to pack, ship, store, and deliver content
https://github.com/docker/distribution
Apache 2 license https://github.com/docker/distribution/blob/master/LICENSE

Moby Project - a collaborative project for the container ecosystem to assemble container-based systems
https://github.com/docker/docker
Copyright 2012-2017 Docker, Inc.
Apache 2 license https://github.com/moby/moby/blob/master/NOTICE

go-connections - Utility package to work with network connections
https://github.com/docker/go-connections
Apache 2 license https://github.com/docker/go-connections/blob/master/LICENSE

go-units - Parse and print size and time units in human-readable format
https://github.com/docker/go-units
Apache 2 license https://github.com/docker/go-units/blob/master/LICENSE

Docker networking
https://github.com/docker/libnetwork
Apache 2 license https://github.com/docker/libnetwork/blob/master/LICENSE

libtrust - Primitives for identity and authorization
https://github.com/docker/libtrust
Copyright 2014 Docker, Inc.
Apache 2 license https://github.com/docker/libtrust/blob/master/LICENSE

spdystream
https://github.com/docker/spdystream
Copyright 2014-2015 Docker, Inc.
Apache 2 license https://github.com/docker/spdystream/blob/master/LICENSE

go-bindata-assetfs - Serves embedded files from jteeuwen/go-bindata with net/http
https://github.com/elazarl/go-bindata-assetfs
Copyright (c) 2014, Elazar Leibovich
2 clause BSD license https://github.com/elazarl/go-bindata-assetfs/blob/master/LICENSE

go-restful-swagger12 - Swagger 1.2 extension to the go-restful package
https://github.com/emicklei/go-restful-swagger12
Copyright (c) 2017 Ernest Micklei
MIT license https://github.com/emicklei/go-restful-swagger12/blob/master/LICENSE

go-restful - package for building REST-style Web Services using Google Go
https://github.com/emicklei/go-restful
Copyright (c) 2012,2013 Ernest Micklei
MIT license https://github.com/emicklei/go-restful/blob/master/LICENSE

go-kmsg-parser - A simpler parser for the /dev/kmsg format
https://github.com/euank/go-kmsg-parser
Copyright 2016 Euan Kemp
Apache 2 license https://github.com/euank/go-kmsg-parser/blob/master/LICENSE

json-patch - A Go library to apply RFC6902 patches and create and apply RFC7386 patches
https://github.com/evanphx/json-patch
Copyright (c) 2014, Evan Phoenix
3 clause BSD license

jsonpath - Extends the Go runtime's json.Decoder enabling navigation of a stream of json tokens.
https://github.com/exponent-io/jsonpath
Copyright (c) 2015 Exponent Labs LLC
MIT license https://github.com/exponent-io/jsonpath/blob/master/LICENSE

camelcase - Split a camelcase word into a slice of words in Go
https://github.com/fatih/camelcase
Copyright (c) 2015 Fatih Arslan
MIT license https://github.com/fatih/camelcase/blob/master/LICENSE.md

fsnotify - Cross-platform file system notifications for Go.
https://github.com/fsnotify/fsnotify
Copyright (c) 2012 The Go Authors. All rights reserved.
Copyright (c) 2012 fsnotify Authors. All rights reserved.
3 clause BSD license https://github.com/fsnotify/fsnotify/blob/master/LICENSE

gardener machine-controller-manager - Declarative way of managing machines for Kubernetes cluster
https://github.com/gardener/machine-controller-manager
Copyright (c) 2017-2018 SAP SE or an SAP affiliate company. All rights reserved.
Apache 2 license https://github.com/gardener/machine-controller-manager/blob/master/LICENSE.md

yaml - A better way to marshal and unmarshal YAML in Golang
https://github.com/ghodss/yaml
Copyright (c) 2014 Sam Ghods
Copyright (c) 2012 The Go Authors. All rights reserved.
MIT License and 3 clause BSD license https://github.com/ghodss/yaml/blob/master/LICENSE

ini - Package ini provides INI file read and write functionality in Go.
https://github.com/go-ini/ini
Apache 2 license

jsonpointer - fork of gojsonpointer with support for structs
https://github.com/go-openapi/jsonpointer
Copyright 2013 sigu-399 ( https://github.com/sigu-399 )
Apache 2 license https://github.com/go-openapi/jsonpointer/blob/master/LICENSE

jsonreference - fork of gojsonreference with support for structs
Copyright 2013 sigu-399 ( https://github.com/sigu-399 )
Apache 2 license https://github.com/go-openapi/jsonreference/blob/master/LICENSE

go-openapi spec - openapi specification object model
https://github.com/go-openapi/spec
Copyright 2015 go-swagger maintainers
Apache 2 license https://github.com/go-openapi/spec/blob/master/LICENSE

go-openapi swag - goodie bag in use in the go-openapi projects
https://github.com/go-openapi/swag
Copyright 2015 go-swagger maintainers
Apache 2 license https://github.com/go-openapi/swag/blob/master/LICENSE

godbus dbus - Native Go bindings for D-Bus
Copyright (c) 2013, Georg Reinke (<guelfey at gmail dot com>), Google
2 clause BSD license https://github.com/godbus/dbus/blob/master/LICENSE

protobuf - Protocol Buffers for Go with Gadgets
https://github.com/gogo/protobuf
Copyright (c) 2013, The GoGo Authors. All rights reserved.
3 clause BSD license https://github.com/gogo/protobuf/blob/master/LICENSE

glog - Leveled execution logs for Go
https://github.com/golang/glog
Copyright 2013 Google Inc. All Rights Reserved.
Apache 2 license https://github.com/golang/glog/blob/master/LICENSE

groupcache - groupcache is a caching and cache-filling library, intended as a replacement for memcached in many cases.
https://github.com/golang/groupcache
Copyright 2012 Google Inc.
Apache 2 license https://github.com/golang/groupcache/blob/master/LICENSE

golang mock - GoMock is a mocking framework for the Go programming language.
https://github.com/golang/mock
Copyright 2012 Google Inc.
Apache 2 license

golang protobuf - Go support for Google's protocol buffers
https://github.com/golang/protobuf
Copyright 2010 The Go Authors.  All rights reserved.
3 clause BSD license https://github.com/golang/protobuf/blob/master/LICENSE

google btree
https://github.com/google/btree
Copyright 2014 Google Inc.
Apache 2 license https://github.com/google/btree/blob/master/LICENSE

google cadvisor - Analyzes resource usage and performance characteristics of running containers.
https://github.com/google/cadvisor
Copyright 2014 The cAdvisor Authors
Apache 2 license https://github.com/google/cadvisor/blob/master/LICENSE

gofuzz - Fuzz testing for go.
https://github.com/google/gofuzz
Copyright 2014 Google Inc. All rights reserved.
Apache 2 license https://github.com/google/gofuzz/blob/master/LICENSE

gnostic - A compiler for API described by the OpenAPI Specification with plugins for code generation and other API support tasks.
https://github.com/googleapis/gnostic
Copyright 2017 Google Inc. All Rights Reserved.
Apache 2 license https://github.com/googleapis/gnostic/blob/master/LICENSE

Gophercloud: an OpenStack SDK for Go
https://github.com/gophercloud/gophercloud/
Copyright 2012-2013 Rackspace, Inc.
Apache 2 license https://github.com/gophercloud/gophercloud/blob/master/LICENSE

websocket - A WebSocket implementation for Go.
https://github.com/gorilla/websocket
Copyright (c) 2013 The Gorilla WebSocket Authors. All rights reserved.
BSD 2 clause license https://github.com/gorilla/websocket/blob/master/LICENSE

httpcache - A Transport for http.Client that will cache responses according to the HTTP RFC
https://github.com/gregjones/httpcache
Copyright © 2012 Greg Jones (greg.jones@gmail.com)
MIT license https://github.com/gregjones/httpcache/blob/master/LICENSE.txt

Golang LRU cache
https://github.com/hashicorp/golang-lru
Mozilla Public License, version 2.0 https://github.com/hashicorp/golang-lru/blob/master/LICENSE

gopass - getpasswd for Go
https://github.com/howeyc/gopass/blob/master/LICENSE.txt
Copyright (c) 2012 Chris Howey
ISC license https://github.com/howeyc/gopass/blob/master/LICENSE.txt

Mergo: merging Go structs and maps since 2013.
https://github.com/imdario/mergo
Copyright (c) 2013 Dario Castañé. All rights reserved.
Copyright (c) 2012 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/imdario/mergo/blob/master/LICENSE

mousetrap - Detect starting from Windows explorer
https://github.com/inconshreveable/mousetrap/blob/master/LICENSE
Copyright 2014 Alan Shreve
Apache 2 license https://github.com/inconshreveable/mousetrap/blob/master/LICENSE

go-jmespath - Golang implementation of JMESPath.
https://github.com/jmespath/go-jmespath
Copyright 2015 James Saryerwinnie
Apache 2 license https://github.com/jmespath/go-jmespath/blob/master/LICENSE

go json-iterator - A high-performance 100% compatible drop-in replacement of "encoding/json"
https://github.com/json-iterator/go
Copyright (c) 2016 json-iterator
MIT license https://github.com/json-iterator/go/blob/master/LICENSE

osext - Extensions to the standard "os" package. Executable and ExecutableFolder.
https://github.com/kardianos/osext/
Copyright (c) 2012 The Go Authors. All rights reserved.
Apache 2 license https://github.com/kardianos/osext/blob/master/LICENSE

fs - Package fs provides filesystem-related functions.
https://github.com/kr/fs
Copyright (c) 2012 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/kr/fs/blob/main/LICENSE

PTY interface for Go
https://github.com/kr/pty/
Copyright (c) 2011 Keith Rarick
MIT license https://github.com/kr/pty/blob/master/License

openstorage - A multi-host clustered implementation of the open storage specification
https://github.com/libopenstorage/openstorage
Copyright 2015 Openstorage.org
Apache 2 license https://github.com/libopenstorage/openstorage/blob/master/LICENSE

godbc - Design by contract for Go
https://github.com/lpabon/godbc
Copyright (c) 2014 The godbc Authors
Apache 2 license https://github.com/lpabon/godbc/blob/master/LICENSE

easyjson - Fast JSON serializer for golang
https://github.com/mailru/easyjson
Copyright (c) 2016 Mail.Ru Group
MIT license https://github.com/mailru/easyjson/blob/master/LICENSE

guid - A Go implementation of Guids as seen in Microsoft's .NET Framework
https://github.com/marstr/guid
Copyright (c) 2016 Martin Strobel
MIT license https://github.com/marstr/guid/blob/master/LICENSE.txt

golang_protobuf_extensions - Support for streaming Protocol Buffer messages for the Go language (golang).
Copyright 2016 Matt T. Proud
Apache 2 license https://github.com/matttproud/golang_protobuf_extensions/blob/master/LICENSE

dns - DNS library in Go
https://github.com/miekg/dns
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/miekg/dns/blob/master/LICENSE

onvml - NVIDIA Management Library (NVML) bindings for Go
https://github.com/mindprince/gonvml
Copyright 2017 Google Inc.
Apache 2 license https://github.com/mindprince/gonvml/blob/master/LICENSE

go-zfs - Go wrappers for ZFS commands
https://github.com/mistifyio/go-zfs
Apache 2 license https://github.com/mistifyio/go-zfs/blob/master/LICENSE

go-wordwrap - A Go (golang) library for wrapping words in a string.
https://github.com/mitchellh/go-wordwrap
Copyright (c) 2014 Mitchell Hashimoto
MIT license https://github.com/mitchellh/go-wordwrap/blob/master/LICENSE.md

mapstructure - Go library for decoding generic map values into native Go structures.
https://github.com/mitchellh/mapstructure
Copyright (c) 2013 Mitchell Hashimoto
MIT license https://github.com/mitchellh/mapstructure/blob/master/LICENSE

deepcopy - Deep copy things
https://github.com/mohae/deepcopy
Copyright (c) 2014 Joel
MIT license https://github.com/mohae/deepcopy/blob/master/LICENSE

fileutils - collection of utilities for file manipulation in golang
https://github.com/mrunalp/fileutils/
Apache 2 license https://github.com/mrunalp/fileutils/blob/master/LICENSE

go-digest - Common digest package used across the container ecosystem
https://github.com/opencontainers/go-digest
Copyright 2016 Docker, Inc.
Apache 2 license https://github.com/opencontainers/go-digest/blob/master/LICENSE

image-spec - OCI Image Format
https://github.com/opencontainers/image-spec
Copyright 2016 The Linux Foundation.
Apache 2 license https://github.com/opencontainers/image-spec/blob/master/LICENSE

runc - CLI tool for spawning and running containers according to the OCI specification
https://github.com/opencontainers/runc
Copyright 2014 Docker, Inc.
Apache 2 license https://github.com/opencontainers/runc/blob/master/LICENSE

runtime-spec - OCI Runtime Specification
https://github.com/opencontainers/runtime-spec
Copyright 2015 The Linux Foundation.
Apache 2 license https://github.com/opencontainers/runtime-spec/blob/master/LICENSE

selinux - common selinux implementation
https://github.com/opencontainers/selinux
Apache 2 license https://github.com/opencontainers/selinux/blob/master/LICENSE

uuid - Automatically exported from code.google.com/p/go-uuid
https://github.com/pborman/uuid
Copyright (c) 2009,2014 Google Inc. All rights reserved.
3 clause BSD license https://github.com/pborman/uuid/blob/master/LICENSE

diskv - A disk-backed key-value store.
https://github.com/peterbourgon/diskv
Copyright (c) 2011-2012 Peter Bourgon
MIT license https://github.com/peterbourgon/diskv/blob/master/LICENSE

errors - Simple error handling primitives
https://github.com/pkg/errors
Copyright (c) 2015, Dave Cheney <dave@cheney.net>
BSD 2 clause license https://github.com/pkg/errors/blob/master/LICENSE

SFTP support for the go.crypto/ssh package
https://github.com/pkg/sftp
Copyright (c) 2013, Dave Cheney
BSD 2 clause license https://github.com/pkg/sftp/blob/master/LICENSE

go-difflib - Partial port of Python difflib package to Go
https://github.com/pmezard/go-difflib
Copyright (c) 2013, Patrick Mezard
3 clause BSD license https://github.com/pmezard/go-difflib/blob/master/LICENSE

prometheus client_golang - Prometheus instrumentation library for Go applications
https://github.com/prometheus/client_golang
Copyright 2015 The Prometheus Authors
Apache 2 license https://github.com/prometheus/client_golang/blob/master/LICENSE

prometheus client_model - Data model artifacts for Prometheus.
https://github.com/prometheus/client_model
Copyright 2013 Prometheus Team
Apache 2 license https://github.com/prometheus/client_model/blob/master/LICENSE

prometheus common - Go libraries shared across Prometheus components and libraries.
https://github.com/prometheus/common
Copyright 2015 The Prometheus Authors
Apache 2 license https://github.com/prometheus/common/blob/master/LICENSE

procfs - provides functions to retrieve system, kernel and process metrics from the pseudo-filesystem proc
https://github.com/prometheus/procfs
Copyright 2014-2015 The Prometheus Authors
Apache 2 license https://github.com/prometheus/procfs/blob/master/LICENSE

Quobyte API Clients
https://github.com/quobyte/api
Copyright (c) 2016, Quobyte Inc.
3 clause BSD license https://github.com/quobyte/api/blob/master/LICENSE

go-rancher - Go language bindings for Rancher API
https://github.com/rancher/go-rancher
Apache 2 license https://github.com/rancher/go-rancher/blob/master/LICENSE

dedent - Remove any common leading whitespace from multiline strings
https://github.com/renstrom/dedent
Copyright (c) 2015 Peter Renström
MIT license https://github.com/renstrom/dedent/blob/master/LICENSE

go-vhd - Go package and CLI to work with VHD images
https://github.com/rubiojr/go-vhd
Copyright (c) 2015 Sergio Rubio
MIT license https://github.com/rubiojr/go-vhd/blob/master/LICENSE

Blackfriday: a markdown processor for Go
https://github.com/russross/blackfriday/
Copyright © 2011 Russ Ross
2 clause BSD license https://github.com/russross/blackfriday/blob/master/LICENSE.txt

uuid - UUID package for Go
https://github.com/satori/go.uuid
Copyright (C) 2013-2018 by Maxim Bublis <b@codemonkey.ru>
MIT license https://github.com/satori/go.uuid/blob/master/LICENSE

The libseccomp golang bindings repository
https://github.com/seccomp/libseccomp-golang
Copyright (c) 2015 Matthew Heon <mheon@redhat.com>
Copyright (c) 2015 Paul Moore <pmoore@redhat.com>
MIT license https://github.com/seccomp/libseccomp-golang/blob/master/LICENSE

sanitized_anchor_name - Package sanitized_anchor_name provides a func to create sanitized anchor names.
https://github.com/shurcooL/sanitized_anchor_name
Copyright (c) 2015 Dmitri Shuralyov
MIT license https://github.com/shurcooL/sanitized_anchor_name/blob/master/LICENSE

logrus - Structured, pluggable logging for Go.
https://github.com/sirupsen/logrus
Copyright (c) 2014 Simon Eskildsen
MIT license https://github.com/sirupsen/logrus/blob/master/LICENSE

afero - A FileSystem Abstraction System for Go
https://github.com/spf13/afero
Copyright © 2016 Steve Francia <spf@spf13.com>.
Apache 2 license https://github.com/spf13/afero/blob/master/LICENSE.txt

cobra - A Commander for modern Go CLI interactions
https://github.com/spf13/cobra
Copyright © 2013 Steve Francia <spf@spf13.com>.
Apache 2 license https://github.com/spf13/cobra/blob/master/LICENSE.txt

pflag - Drop-in replacement for Go's flag package, implementing POSIX/GNU-style --flags.
https://github.com/spf13/pflag
Copyright (c) 2012 Alex Ogier. All rights reserved.
Copyright (c) 2012 The Go Authors. All rights reserved.
BSD 3 clause license https://github.com/spf13/pflag/blob/master/LICENSE

objx - Go package for dealing with maps, slices, JSON and other data.
https://github.com/stretchr/objx
Copyright (c) 2014 Stretchr, Inc.
Copyright (c) 2017-2018 objx contributors
MIT license https://github.com/stretchr/objx/blob/master/LICENSE

testify - A toolkit with common assertions and mocks that plays nicely with the standard library
https://github.com/stretchr/testify
Copyright (c) 2012 - 2013 Mat Ryer and Tyler Bunnell
MIT license https://github.com/stretchr/testify/blob/master/LICENSE

gocapability - Utilities for manipulating POSIX capabilities in Go.
https://github.com/syndtr/gocapability
Copyright 2013 Suryandaru Triandana <syndtr@gmail.com>
All rights reserved.
3 clause BSD license https://github.com/syndtr/gocapability/blob/master/LICENSE

ugorji go - idiomatic codec and rpc lib for msgpack, cbor, json, etc. msgpack.org[Go]
https://github.com/ugorji/go
Copyright (c) 2012-2015 Ugorji Nwoke.
All rights reserved.
MIT license https://github.com/ugorji/go/blob/master/LICENSE

netlink - Simple netlink library for go.
https://github.com/vishvananda/netlink
Copyright 2014 Vishvananda Ishaya.
Copyright 2014 Docker, Inc.
Apache 2 license https://github.com/vishvananda/netlink/blob/master/LICENSE

netns - Simple network namespace handling for go.
https://github.com/vishvananda/netns
Copyright 2014 Vishvananda Ishaya.
Copyright 2014 Docker, Inc.
Apache 2 license https://github.com/vishvananda/netns/blob/master/LICENSE

govmomi - Go library for the VMware vSphere API
https://github.com/vmware/govmomi
Copyright (c) 2015-2017 VMware, Inc. All Rights Reserved.
Apache 2 license https://github.com/vmware/govmomi/blob/master/LICENSE.txt

photon-controller-go-sdk
https://github.com/vmware/photon-controller-go-sdk
Copyright (c) 2016 VMware, Inc. All Rights Reserved.
Apache 2 license https://github.com/vmware/photon-controller-go-sdk/blob/master/LICENSE

go-cloudstack - A CloudStack API client enabling Go programs to interact with CloudStack in a simple and uniform way
https://github.com/xanzy/go-cloudstack
Copyright 2018, Sander van Harmelen
Apache 2 license https://github.com/xanzy/go-cloudstack/blob/master/LICENSE

go4.org
https://github.com/go4org/go4/
Copyright 2015 The Go4 Authors
Apache 2 license https://github.com/go4org/go4/blob/master/LICENSE

crypto - [mirror] Go supplementary cryptography libraries
https://github.com/golang/crypto/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/crypto/blob/master/LICENSE

exp - [mirror] Experimental and deprecated packages
https://github.com/golang/exp/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/exp/blob/master/LICENSE

net - [mirror] Go supplementary network libraries
https://github.com/golang/net/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/net/blob/master/LICENSE

Go OAuth2
https://github.com/golang/oauth2/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/oauth2/blob/master/LICENSE

sys - [mirror] Go packages for low-level interaction with the operating system
https://github.com/golang/sys/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/sys/blob/master/LICENSE

text - [mirror] Go text processing support
https://github.com/golang/text/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/text/blob/master/LICENSE

time - [mirror] Go supplementary time packages
https://github.com/golang/time/
Copyright (c) 2009 The Go Authors. All rights reserved.
3 clause BSD license https://github.com/golang/time/blob/master/LICENSE

google-api-go-client - Auto-generated Google APIs for Go
https://github.com/google/google-api-go-client
Copyright (c) 2011 Google Inc. All rights reserved.
3 clause BSD license https://github.com/google/google-api-go-client/blob/master/LICENSE

go-genproto
https://github.com/google/go-genproto
Copyright 2016 Google Inc.
Apache 2 license https://github.com/google/go-genproto/blob/master/LICENSE

grpc-go - The Go language implementation of gRPC. HTTP/2 based RPC
https://github.com/grpc/grpc-go
Copyright 2017 gRPC authors.
Apache 2 license https://github.com/grpc/grpc-go/blob/master/LICENSE

gcfg - read INI-style configuration files into Go structs; supports user-defined types and subsections
https://github.com/go-gcfg/gcfg/
Copyright (c) 2012 Péter Surányi. Portions Copyright (c) 2009 The Go
Authors. All rights reserved.
3 clause BSD license https://github.com/go-gcfg/gcfg/blob/v1/LICENSE

inf - Package inf (type inf.Dec) implements "infinite-precision" decimal arithmetic.
https://github.com/go-inf/inf
Copyright (c) 2012 Péter Surányi. Portions Copyright (c) 2009 The Go
Authors. All rights reserved.
3 clause BSD license https://github.com/go-inf/inf/blob/master/LICENSE

go-jose - An implementation of JOSE standards (JWE, JWS, JWT) in Go
https://github.com/square/go-jose
Copyright 2014 Square Inc.
Apache 2 license https://github.com/square/go-jose/blob/master/LICENSE

warnings - Package warnings implements error handling with non-fatal errors (warnings).
https://github.com/go-warnings/warnings
Copyright (c) 2016 Péter Surányi.
2 clause BSD license https://github.com/go-warnings/warnings/blob/master/LICENSE

yaml - YAML support for the Go language.
https://github.com/go-yaml/yaml
Copyright 2011-2016 Canonical Ltd.
Copyright (c) 2006 Kirill Simonov
Apache 2 license https://github.com/go-yaml/yaml/blob/v2/LICENSE
some files are MIT license https://github.com/go-yaml/yaml/blob/v2/LICENSE.libyaml

kubernetes api - The canonical location of the Kubernetes API definition.
https://github.com/kubernetes/api
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/api/blob/master/LICENSE

kubernetes API server for API extensions like CustomResourceDefinitions
https://github.com/kubernetes/apiextensions-apiserver
Copyright 2017 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/apiextensions-apiserver/blob/master/LICENSE

kubernetes apimachinery
https://github.com/kubernetes/apimachinery
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/apimachinery/blob/master/LICENSE

kubernetes apiserver - Library for writing a Kubernetes-style API server.
https://github.com/kubernetes/apiserver
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/apiserver/blob/master/LICENSE

Go client for Kubernetes.
https://github.com/kubernetes/client-go
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/client-go/blob/master/LICENSE

Kubernetes OpenAPI spec generation & serving
https://github.com/kubernetes/kube-openapi
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/kube-openapi/blob/master/LICENSE

kubernetes - Production-Grade Container Scheduling and Management
https://github.com/kubernetes/kubernetes
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/kubernetes/blob/master/LICENSE

utils - Non-Kubernetes-specific utility libraries which are consumed by multiple projects.
https://github.com/kubernetes/utils
Copyright 2018 The Kubernetes Authors.
Apache 2 license https://github.com/kubernetes/utils/blob/master/LICENSE

util - Go utility packages
https://github.com/fvbommel/util
Copyright (c) 2015 Frits van Bommel
MIT license https://github.com/fvbommel/util/blob/master/LICENSE

machine-controller-manager
https://github.com/gardener/machine-controller-manager
Copyright (c) 2017 SAP SE or an SAP affiliate company.
Apache 2 license https://github.com/gardener/machine-controller-manager/blob/master/LICENSE.md`,
	},
	{
		// From https://github.com/apache/mynewt-artifact/blob/v0.0.15/LICENSE
		Name: "Apache-2.0",
		Text: `
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "{}"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright {yyyy} {name of copyright owner}

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

This product bundles pretty, which is available under the MIT license.  For
details, see:
     * newt/vendor/github.com/kr/pretty/License
     * newtmgr/vendor/github.com/kr/pretty/License

This product bundles kr/text, which is available under the MIT license.  For
details, see:
     * newt/vendor/github.com/kr/text/License
     * newtmgr/vendor/github.com/kr/text/License

This product bundles mapstructure, which is available under the MIT license.
For details, see:
    * newt/vendor/github.com/mitchellh/mapstructure/LICENSE
    * newtmgr/vendor/github.com/mitchellh/mapstructure/LICENSE

This product bundles logrus, which is available under the MIT license.  For
details, see:
    * newt/vendor/github.com/sirupsen/logrus/LICENSE
    * newtmgr/vendor/github.com/sirupsen/logrus/LICENSE

This product bundles Cast, which is available under the MIT license.  For
details, see:
    * newt/vendor/github.com/spf13/cast/LICENSE
    * newtmgr/vendor/github.com/spf13/cast/LICENSE

This product bundles jWalterWeatherman, which is available under the MIT
license.  For details, see:
    * newt/vendor/github.com/spf13/jwalterweatherman/LICENSE
    * newtmgr/vendor/github.com/spf13/jwalterweatherman/LICENSE

This product bundles pflag, which is available under the "3-clause BSD"
license.  For details, see:
    * newt/vendor/github.com/spf13/pflag/LICENSE
    * newtmgr/vendor/github.com/spf13/pflag/LICENSE

This product bundles the unix Go package, which is available under the
"3-clause BSD" license.  For details, see:
    * newt/vendor/golang.org/x/sys/LICENSE
    * newtmgr/vendor/golang.org/x/sys/LICENSE

This product bundles fsnotify.v1, which is available under the "3-clause BSD"
license.  For details, see:
    * newt/vendor/gopkg.in/fsnotify.v1/LICENSE
    * newtmgr/vendor/gopkg.in/fsnotify.v1/LICENSE

This product bundles yaml.v2's Go port of libyaml, which is available under the
MIT license.  For details, see:
    * newt/vendor/mynewt.apache.org/newt/yaml/apic.go
    * newt/vendor/mynewt.apache.org/newt/yaml/emitterc.go
    * newt/vendor/mynewt.apache.org/newt/yaml/parserc.go
    * newt/vendor/mynewt.apache.org/newt/yaml/readerc.go
    * newt/vendor/mynewt.apache.org/newt/yaml/scannerc.go
    * newt/vendor/mynewt.apache.org/newt/yaml/writerc.go
    * newt/vendor/mynewt.apache.org/newt/yaml/yamlh.go
    * newt/vendor/mynewt.apache.org/newt/yaml/yamlprivateh.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/apic.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/emitterc.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/parserc.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/readerc.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/scannerc.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/writerc.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/yamlh.go
    * newtmgr/vendor/mynewt.apache.org/newt/yaml/yamlprivateh.go
    * yaml/apic.go
    * yaml/emitterc.go
    * yaml/parserc.go
    * yaml/readerc.go
    * yaml/scannerc.go
    * yaml/writerc.go
    * yaml/yamlh.go
    * yaml/yamlprivateh.go

This product bundles viper, which is available under the MIT license.  For
details, see:
    * newt/vendor/mynewt.apache.org/newt/viper/LICENSE
    * newtmgr/vendor/mynewt.apache.org/newt/viper/LICENSE
    * viper/LICENSE

This product bundles go-crc16, which is available under the MIT license.  For
details, see:
    * newtmgr/vendor/github.com/joaojeronimo/go-crc16/README.md

This product bundles GATT, which is available under the "3-clause BSD" license.
For details, see:
    * newtmgr/vendor/github.com/runtimeinc/gatt

This product bundles xpc, which is available under the MIT license.  For
details, see:
    * newtmgr/vendor/github.com/runtimeinc/gatt/xpc/LICENSE

This product bundles gioctl, which is available under the MIT license.  For
details, see:
    * newtmgr/vendor/github.com/runtimeinc/gatt/linux/gioctl/LICENSE.md

This product bundles tarm/serial, which is available under the "3-clause BSD"
license.  For details, see:
    * newtmgr/vendor/github.com/tarm/serial/LICENSE

This product bundles ugorji/go/codec, which is available under the MIT license.
For details, see:
    * newtmgr/vendor/github.com/ugorji/go/LICENSE

This product bundles go-coap which is available under the MIT license.
For details, see:
    * newtmgr/vendor/github.com/dustin/go-coap/LICENSE

This product bundles go-homedir which is available under the MIT license.
For details, see:
    * newtmgr/vendor/github.com/mitchellh/go-homedir/LICENSE`,
	},
	{
		// From https://github.com/apache/mynewt-newtmgr/blob/master/LICENSE.
		Name: "Apache-2.0",
		Text: `
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "{}"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright {yyyy} {name of copyright owner}

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

This product bundles pretty, which is available under the MIT license. For
details, see:
    * vendor/github.com/kr/pretty/License

This product bundles kr/text, which is available under the MIT license. For
details, see:
    * vendor/github.com/kr/text/License

This product bundles mapstructure, which is available under the MIT license.
For details, see:
    * vendor/github.com/mitchellh/mapstructure/LICENSE

This product bundles logrus, which is available under the MIT license. For
details, see:
    * vendor/github.com/Sirupsen/logrus/LICENSE

This product bundles Cast, which is available under the MIT license. For
details, see:
    * vendor/github.com/spf13/cast/LICENSE

This product bundles jWalterWeatherman, which is available under the MIT
license. For details, see:
    * vendor/github.com/spf13/jwalterweatherman/LICENSE

This product bundles pflag, which is available under the "3-clause BSD"
license. For details, see:
    * vendor/github.com/spf13/pflag/LICENSE

This product bundles the unix Go package, which is available under the
"3-clause BSD" license. For details, see:
    * vendor/golang.org/x/sys/LICENSE

This product bundles the Go supplementary cryptography libraries package,
which is available under the "3-clause BSD" license. For details, see:
    * vendor/golang.org/x/crypto/LICENSE

This product bundles the Go supplementary networking libraries package, which
is available under the "3-clause BSD" license. For details, see:
    * vendor/golang.org/x/net/LICENSE

This product bundles fsnotify.v1, which is available under the "3-clause BSD"
license. For details, see:
    * vendor/gopkg.in/fsnotify.v1/LICENSE

This product bundles yaml.v2's Go port of libyaml, which is available under the
MIT license. For details, see:
    * vendor/mynewt.apache.org/newt/yaml/apic.go
    * vendor/mynewt.apache.org/newt/yaml/emitterc.go
    * vendor/mynewt.apache.org/newt/yaml/parserc.go
    * vendor/mynewt.apache.org/newt/yaml/readerc.go
    * vendor/mynewt.apache.org/newt/yaml/scannerc.go
    * vendor/mynewt.apache.org/newt/yaml/writerc.go
    * vendor/mynewt.apache.org/newt/yaml/yamlh.go
    * vendor/mynewt.apache.org/newt/yaml/yamlprivateh.go

This product bundles viper, which is available under the MIT license. For
details, see:
    * vendor/mynewt.apache.org/newt/viper/LICENSE

This product bundles pb, which is available under the "3-clause BSD" license.
For details, see:
    * vendor/github.com/cheggaaa/pb/LICENSE

This product bundles ble, which is available under the "3-clause BSD" license.
For details, see:
    * vendor/github.com/currantlabs/ble/LICENSE

This product bundles structs, which is available under the MIT license. For
details, see:
    * vendor/github.com/fatih/structs/LICENSE

This product bundles structs, which is available under the Apache License 2.0.
For details, see:
    * vendor/github.com/inconshreveable/mousetrap/LICENSE

This product bundles go-crc16, which is available under the MIT license. For
details, see:
    * vendor/github.com/joaojeronimo/go-crc16/README.md

This product bundles ansi, which is available under the MIT license. For
details, see:
    * vendor/github.com/mgutz/ansi/LICENSE

This product bundles logxi, which is available under the MIT license. For
details, see:
    * vendor/github.com/mgutz/logxi/LICENSE

This product bundles go-homedir, which is available under the MIT license. For
details, see:
    * vendor/github.com/mitchellh/go-homedir/LICENSE

This product bundles go-codec, which is available under the MIT license. For
details, see:
    * vendor/github.com/ugorji/go/LICENSE

This product bundles goble, which is available under the MIT license. For
details, see:
    * vendor/github.com/raff/goble/LICENSE

This product bundles go-coap, which is available under the MIT license. For
details, see:
    * vendor/github.com/runtimeco/go-coap/LICENSE

This product bundles structs, which is available under the Apache License 2.0.
For details, see:
    * vendor/github.com/spf13/cobra/LICENSE.txt

This product bundles serial, which is available under the "3-clause BSD"
license. For details, see:
    * vendor/github.com/tarm/serial/LICENSE

This product bundles go-colorable, which is available under the MIT license.
For details, see:
    * vendor/github.com/mattn/go-colorable/LICENSE

This product bundles go-runewidth, which is available under the MIT license.
For details, see:
    * vendor/github.com/mattn/go-runewidth/LICENSE

This product bundles go-isatty, which is available under the MIT license.
For details, see:
    * vendor/github.com/mattn/go-isatty/LICENSE

This product bundles readline, which is available under the MIT license.
For details, see:
    * vendor/github.com/chzyer/readline/LICENSE

This product bundles go-blel, which is available under the "3-clause BSD"
license. For details, see:
    * vendor/github.com/go-ble/ble/LICENSE

This product bundles ishell.v1, which is available under the MIT license.
For details, see:
    * vendor/gopkg.in/abiosoft/ishell.v1/LICENSE`,
	},
}
