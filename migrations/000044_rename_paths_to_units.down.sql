-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units RENAME TO paths;

CREATE VIEW units AS SELECT * FROM paths;

ALTER INDEX units_pkey RENAME TO paths_pkey;
ALTER INDEX units_path_module_id_key RENAME TO paths_path_module_id_key;
ALTER INDEX idx_units_module_id RENAME TO idx_paths_module_id;
ALTER INDEX idx_units_v1_path RENAME TO idx_paths_v1_path;

COMMENT ON TABLE paths IS
'TABLE paths contains every module, package and directory path at every version.';

END;
