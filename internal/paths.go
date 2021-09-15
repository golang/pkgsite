// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"path"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal/stdlib"
)

var vcsHostsWithThreeElementRepoName = map[string]bool{
	"bitbucket.org": true,
	"gitea.com":     true,
	"gitee.com":     true,
	"github.com":    true,
	"gitlab.com":    true,
}

// CandidateModulePaths returns the potential module paths that could contain
// the fullPath, from longest to shortest. It returns nil if no valid module
// paths can be constructed.
func CandidateModulePaths(fullPath string) []string {
	if stdlib.Contains(fullPath) {
		if err := module.CheckImportPath(fullPath); err != nil {
			return nil
		}
		return []string{"std"}
	}
	var r []string
	for p := fullPath; p != "." && p != "/"; p = path.Dir(p) {
		if err := module.CheckPath(p); err != nil {
			continue
		}
		r = append(r, p)
	}
	if len(r) == 0 {
		return nil
	}
	if !vcsHostsWithThreeElementRepoName[r[len(r)-1]] {
		return r
	}
	if len(r) < 3 {
		return nil
	}
	return r[:len(r)-2]
}
