// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
)

func insertSymbols(ctx context.Context, db *database.DB, pathToDocs map[string][]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertSymbols")

	var symNames []interface{}
	for _, docs := range pathToDocs {
		for _, doc := range docs {
			for _, s := range doc.API {
				symNames = append(symNames, s.Name)
				if len(s.Children) > 0 {
					for _, s := range s.Children {
						symNames = append(symNames, s.Name)
					}
				}
			}
		}
	}
	_, err = upsertSymbolsReturningIDs(ctx, db, symNames)
	return err
}

func upsertSymbolsReturningIDs(ctx context.Context, db *database.DB, values []interface{}) (map[string]int, error) {
	if err := db.BulkInsert(ctx, "symbols", []string{"name"}, values, database.OnConflictDoNothing); err != nil {
		return nil, err
	}
	query := `
        SELECT id, name
        FROM symbols
        WHERE name = ANY($1);`
	symbols := map[string]int{}
	collect := func(rows *sql.Rows) error {
		var name string
		var id int
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		symbols[name] = id
		if id == 0 {
			return fmt.Errorf("id can't be 0: %q", name)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, pq.Array(values)); err != nil {
		return nil, err
	}
	return symbols, nil
}
