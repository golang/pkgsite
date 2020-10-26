// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// All of these RemoveNonRedistributableData methods remove data that we cannot
// legally redistribute if the receiver is non-redistributable.

package internal

func (m *Module) RemoveNonRedistributableData() {
	for _, l := range m.Licenses {
		l.RemoveNonRedistributableData()
	}
	for _, d := range m.Units {
		d.RemoveNonRedistributableData()
	}
}

func (u *Unit) RemoveNonRedistributableData() {
	if !u.IsRedistributable {
		u.Readme = nil
		u.Documentation = nil
	}
}

func (p *PackageMeta) RemoveNonRedistributableData() {
	if !p.IsRedistributable {
		p.Synopsis = ""
	}
}
