// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"context"
	"io/fs"
)

// A Detector detects licenses in a module and its packages.
type Detector interface {
	ModuleIsRedistributable(ctx context.Context) bool
	AllLicenses(ctx context.Context) []*License
	PackageInfo(ctx context.Context, dir string) (isRedistributable bool, lics []*License)
}

// DetectorFunc constructs a Detector for the given module, version, and
// module content filesystem.
type DetectorFunc func(ctx context.Context, modulePath, version string, fsys fs.FS) Detector
