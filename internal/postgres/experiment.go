// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// GetExperiments fetches all experiments in the database.
func (db *DB) GetExperiments(ctx context.Context) (_ []*internal.Experiment, err error) {
	defer derrors.Wrap(&err, "DB.GetExperiments(ctx)")

	query := "SELECT name, rollout, description FROM experiments;"
	var experiments []*internal.Experiment
	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var e internal.Experiment
		if err := rows.Scan(&e.Name, &e.Rollout, &e.Description); err != nil {
			return err
		}
		experiments = append(experiments, &e)
		return nil
	})
	return experiments, nil
}

// InsertExperiment inserts a row into the experiments table.
func (db *DB) InsertExperiment(ctx context.Context, e *internal.Experiment) (err error) {
	defer derrors.Wrap(&err, "DB.insertExperiment(ctx, %v)", e)
	if e.Name == "" || e.Description == "" {
		return fmt.Errorf("neither name nor description can be empty: %w", derrors.InvalidArgument)
	}

	_, err = db.db.Exec(ctx,
		`INSERT INTO experiments
		(name, rollout, description) VALUES ($1, $2, $3);`,
		e.Name, e.Rollout, e.Description)
	return err
}

// UpdateExperiment updates the specified experiment with the provided rollout value.
func (db *DB) UpdateExperiment(ctx context.Context, e *internal.Experiment) (err error) {
	defer derrors.Wrap(&err, "DB.UpdateExperiment(ctx, %v)", e)
	if e.Name == "" || e.Description == "" {
		return fmt.Errorf("neither name nor description can be empty: %w", derrors.InvalidArgument)
	}

	query := `UPDATE experiments
		SET rollout = $2, description = $3
		WHERE name = $1;`
	_, err = db.db.Exec(ctx, query, e.Name, e.Rollout, e.Description)
	return err
}

// RemoveExperiment removes the specified experiment.
func (db *DB) RemoveExperiment(ctx context.Context, name string) (err error) {
	defer derrors.Wrap(&err, "DB.RemoveExperiment(ctx, %q)", name)
	res, err := db.db.Exec(ctx, `DELETE FROM experiments WHERE name = $1`, name)
	if err != nil {
		return err
	}
	return notFoundIfNoRows(res)
}

func notFoundIfNoRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RowsAffected: %w", err)
	}
	if n == 0 {
		return derrors.NotFound
	}
	return nil
}
