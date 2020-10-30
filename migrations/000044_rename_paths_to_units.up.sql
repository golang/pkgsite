-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP VIEW units;

ALTER TABLE paths RENAME TO units;

ALTER INDEX paths_pkey RENAME TO units_pkey;
ALTER INDEX paths_path_module_id_key RENAME TO units_path_module_id_key;
ALTER INDEX idx_paths_module_id RENAME TO idx_units_module_id;
ALTER INDEX idx_paths_v1_path RENAME TO idx_units_v1_path;

COMMENT ON TABLE units IS
'TABLE units contains every module, package and directory path at every version.';

END;
