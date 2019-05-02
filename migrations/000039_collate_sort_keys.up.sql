-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP VIEW IF EXISTS vw_licensed_packages CASCADE;

-- Reorder primary key for imports to reflect sort order
ALTER TABLE imports DROP CONSTRAINT imports_pkey;
ALTER TABLE imports ADD CONSTRAINT imports_pkey PRIMARY KEY (to_path, from_path, from_version);

ALTER TABLE documents ALTER COLUMN module_path TYPE TEXT COLLATE "C";
ALTER TABLE documents ALTER COLUMN package_path TYPE TEXT COLLATE "C";
ALTER TABLE documents ALTER COLUMN series_path TYPE TEXT COLLATE "C";
ALTER TABLE licenses ALTER COLUMN file_path TYPE TEXT COLLATE "C";
ALTER TABLE licenses ALTER COLUMN module_path TYPE TEXT COLLATE "C";
ALTER TABLE licenses ALTER COLUMN module_path TYPE TEXT COLLATE "C";
ALTER TABLE package_licenses ALTER COLUMN module_path TYPE TEXT COLLATE "C";
ALTER TABLE package_licenses ALTER COLUMN package_path TYPE TEXT COLLATE "C";

-- vw_licensed_packages is a helper view that captures package licenses when
-- querying.
-- This view needs to be recreated or we will get errors for trying to alter the column types.
-- "ERROR:  cannot alter type of a column used by a view or rule"
CREATE OR REPLACE VIEW vw_licensed_packages AS
SELECT
  p.*,
  -- Aggregate license information into arrays which can later be zipped
  -- together. The FILTER clause here is necessary due to the left-join.  The
  -- ORDER BY clause should not technically be necessary as ordering should be
  -- stable, but we include it to have deterministic sorting.
  array_agg(l.type ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) license_types,
  array_agg(l.file_path ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) license_paths
FROM
  packages p
LEFT JOIN
  package_licenses pl
ON
  p.module_path = pl.module_path
  AND p.version = pl.version
  AND p.path = pl.package_path
LEFT JOIN
  licenses l
ON
  pl.module_path = l.module_path
  AND pl.version = l.version
  AND pl.file_path = l.file_path
GROUP BY (p.module_path, p.version, p.path);

END;
