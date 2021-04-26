// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/symbol"
)

// GetSymbolHistory returns a map of the first version when a symbol name is
// added to the API, to the symbol name, to the UnitSymbol struct. The
// UnitSymbol.Children field will always be empty, as children names are also
// tracked.
func (db *DB) GetSymbolHistory(ctx context.Context, packagePath, modulePath string,
) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	defer derrors.Wrap(&err, "GetSymbolHistory(ctx, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetSymbolHistory")()

	if experiment.IsActive(ctx, internal.ExperimentReadSymbolHistory) {
		// TODO(https://golang.org/issue/37102): read data from the
		// symbol_history table.
		return nil, nil
	}

	versionToNameToUnitSymbols, err := getPackageSymbols(ctx, db.db, packagePath, modulePath)
	if err != nil {
		return nil, err
	}
	return symbol.IntroducedHistory(versionToNameToUnitSymbols), nil
}
