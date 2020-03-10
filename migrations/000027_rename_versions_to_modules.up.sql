-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP VIEW modules;

ALTER TABLE versions RENAME TO modules;

ALTER INDEX versions_pkey RENAME TO modules_pkey;
ALTER INDEX idx_versions_module_path_text_pattern_ops RENAME TO idx_modules_module_path_text_pattern_ops;
ALTER INDEX idx_versions_sort_version RENAME TO idx_modules_sort_version;
ALTER INDEX idx_versions_version_type RENAME TO idx_modules_version_type;

END;
