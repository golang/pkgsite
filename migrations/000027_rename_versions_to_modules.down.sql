-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules RENAME TO versions;

CREATE VIEW modules AS SELECT * FROM versions;

ALTER INDEX modules_pkey RENAME TO versions_pkey;
ALTER INDEX idx_modules_module_path_text_pattern_ops RENAME TO idx_versions_module_path_text_pattern_ops;
ALTER INDEX idx_modules_sort_version RENAME TO idx_versions_sort_version;
ALTER INDEX idx_modules_version_type RENAME TO idx_versions_version_type;

END;
