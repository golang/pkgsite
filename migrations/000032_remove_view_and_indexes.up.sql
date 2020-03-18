-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_imported_by_count_gt_8;
DROP INDEX idx_imported_by_count_gt_50;
DROP VIEW vw_module_licenses;
DROP FUNCTION all_redistributable;

END;
