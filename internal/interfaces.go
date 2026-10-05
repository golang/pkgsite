// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"time"
)

// PostgresDB provides an interface satisfied by *(internal/postgres.DB) so that
// packages in pkgsite can use the database if it exists without needing a
// dependency on the database driver packages.
type PostgresDB interface {
	DataSource

	IsExcluded(ctx context.Context, path, version string) bool
	GetImportedBy(ctx context.Context, pkgPath, modulePath string, start string, limit int) (paths []string, err error)
	GetImportedByCounts(ctx context.Context, pkgPath, modulePath string) (ImportedByCounts, error)
	GetLatestMajorPathForV1Path(ctx context.Context, v1path string) (_ string, _ int, err error)
	GetStdlibPathsWithSuffix(ctx context.Context, suffix string) (paths []string, err error)
	GetSymbolHistory(ctx context.Context, packagePath, modulePath string) (_ *SymbolHistory, err error)
	GetVersionMap(ctx context.Context, modulePath, requestedVersion string) (_ *VersionMap, err error)
	GetVersionMaps(ctx context.Context, paths []string, requestedVersion string) (_ []*VersionMap, err error)
	InsertModule(ctx context.Context, m *Module, lmv *LatestModuleVersions) (isLatest bool, err error)
	UpsertVersionMap(ctx context.Context, vm *VersionMap) (err error)
	GetVersionsForPath(ctx context.Context, path string) ([]*ModuleInfo, error)
}

// ImportedByCounts contains counts of packages and modules that import a particular package,
// along with the times they were last updated in the DB. A zero time means the count was never
// updated.
type ImportedByCounts struct {
	Packages          int
	PackagesUpdatedAt time.Time
	Modules           int
	ModulesUpdatedAt  time.Time
}
